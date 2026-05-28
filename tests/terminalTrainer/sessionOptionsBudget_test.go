// tests/terminalTrainer/sessionOptionsBudget_test.go
//
// Budget enrichment of GET /terminals/session-options response.
//
// EnrichSessionOptionsBudget stamps the per-size RemainingCount + MemoryMB
// and the top-level Quota envelope on a SessionOptionsResponse. The pure
// ComputeSessionOptions test coverage lives in composedSession_test.go.
package terminalTrainer_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"
)

// budgetSessionOptions builds a baseline SessionOptionsResponse with the
// canonical XS/S/M/L/XL sizes. We don't care about the distribution shape
// here — only the per-size enrichment is exercised.
func budgetSessionOptions() *dto.SessionOptionsResponse {
	sizes := []dto.SessionOptionSize{
		{TTSize: dto.TTSize{Key: "XS", SortOrder: 10}, Allowed: true},
		{TTSize: dto.TTSize{Key: "S", SortOrder: 20}, Allowed: true},
		{TTSize: dto.TTSize{Key: "M", SortOrder: 30}, Allowed: true},
		{TTSize: dto.TTSize{Key: "L", SortOrder: 40}, Allowed: true},
		{TTSize: dto.TTSize{Key: "XL", SortOrder: 50}, Allowed: true},
	}
	return &dto.SessionOptionsResponse{AllowedSizes: sizes}
}

// findSize returns the SessionOptionSize matching the requested key.
func findSize(t *testing.T, opts *dto.SessionOptionsResponse, key string) *dto.SessionOptionSize {
	t.Helper()
	for i := range opts.AllowedSizes {
		if opts.AllowedSizes[i].Key == key {
			return &opts.AllowedSizes[i]
		}
	}
	t.Fatalf("size %q not in response", key)
	return nil
}

// TestSessionOptions_BudgetMode_IncludesRemainingCount — each size's
// RemainingCount must reflect the user's remaining budget.
func TestSessionOptions_BudgetMode_IncludesRemainingCount(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "BudgetEnrich",
		MaxCPU:      8000, // 8 vCPU in mCPU
		MaxMemoryMB: 4096,
	}

	opts := budgetSessionOptions()
	svc.EnrichSessionOptionsBudget(opts, plan, "u-enrich", nil)

	// With 8000 mCPU / 4096 MiB and zero usage (sizes in mCPU):
	//   xs (500/256MiB)   → min(8000/500,  4096/256)  = min(16, 16) = 16
	//   s  (1000/512MiB)  → min(8000/1000, 4096/512)  = min(8, 8)   = 8
	//   m  (2000/1024MiB) → min(8000/2000, 4096/1024) = min(4, 4)   = 4
	//   l  (4000/2048MiB) → min(8000/4000, 4096/2048) = min(2, 2)   = 2
	//   xl (4000/4096MiB) → min(8000/4000, 4096/4096) = min(2, 1)   = 1
	assert.Equal(t, 16, findSize(t, opts, "XS").RemainingCount,
		"XS at 500 mCPU under 8000 mCPU → 16 (twice the old XS=1 result)")
	assert.Equal(t, 8, findSize(t, opts, "S").RemainingCount)
	assert.Equal(t, 4, findSize(t, opts, "M").RemainingCount)
	assert.Equal(t, 2, findSize(t, opts, "L").RemainingCount)
	assert.Equal(t, 1, findSize(t, opts, "XL").RemainingCount)

	// MemoryMB always stamped from the catalog regardless of mode.
	assert.Equal(t, 256, findSize(t, opts, "XS").MemoryMB)
	assert.Equal(t, 4096, findSize(t, opts, "XL").MemoryMB)
}

// TestSessionOptions_BudgetMode_IncludesTopLevelQuota — Quota block must
// carry MaxCPU/MaxMemoryMB, Used*, Remaining* and Scope.
func TestSessionOptions_BudgetMode_IncludesTopLevelQuota(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "BudgetEnrichQuota",
		MaxCPU:      4000, // 4 vCPU in mCPU
		MaxMemoryMB: 2048,
	}

	// Seed an M (2000 mCPU / 1g) so usage is non-zero.
	insertExistingTerminal(t, db, "u-quota", nil, "running", "ephemeral", 2000, 1024)

	opts := budgetSessionOptions()
	svc.EnrichSessionOptionsBudget(opts, plan, "u-quota", nil)

	require.NotNil(t, opts.Quota, "Quota block must be present in budget mode")
	assert.Equal(t, 4000, opts.Quota.MaxCPU)
	assert.Equal(t, 2048, opts.Quota.MaxMemoryMB)
	assert.Equal(t, 2000, opts.Quota.UsedCPU)
	assert.Equal(t, 1024, opts.Quota.UsedMemoryMB)
	assert.Equal(t, 2000, opts.Quota.RemainingCPU)
	assert.Equal(t, 1024, opts.Quota.RemainingMemoryMB)
	assert.Equal(t, "user", opts.Quota.Scope, "personal context → scope=user")
}

// TestSessionOptions_UnlimitedPlan_UnlimitedScope — plans with zero CPU/RAM
// caps emit Scope="unlimited" so the frontend renders an unconstrained UI.
func TestSessionOptions_UnlimitedPlan_UnlimitedScope(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Unlimited",
		// MaxCPU=0 and MaxMemoryMB=0 → unlimited on both axes.
	}

	opts := budgetSessionOptions()
	svc.EnrichSessionOptionsBudget(opts, plan, "u-unlim", nil)

	require.NotNil(t, opts.Quota)
	// MemoryMB stamp is mode-independent.
	assert.Equal(t, 1024, findSize(t, opts, "M").MemoryMB)
}

// TestSessionOptions_BudgetMode_OrgScope — when orgID is non-nil, Scope
// must be "organization" so dashboards can label the budget accordingly.
func TestSessionOptions_BudgetMode_OrgScope(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "OrgBudget",
		MaxCPU:      8000, // 8 vCPU in mCPU
		MaxMemoryMB: 4096,
	}
	orgID := uuid.New()

	opts := budgetSessionOptions()
	svc.EnrichSessionOptionsBudget(opts, plan, "u-org", &orgID)

	require.NotNil(t, opts.Quota)
	assert.Equal(t, "organization", opts.Quota.Scope,
		"non-nil orgID → scope=organization")
}
