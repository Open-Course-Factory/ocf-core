// tests/terminalTrainer/sessionOptionsBudget_test.go
//
// MR-CORE-6 — Budget enrichment of GET /terminals/session-options response.
//
// EnrichSessionOptionsBudget stamps the per-size RemainingCount + MemoryMB
// and the top-level Quota envelope on a SessionOptionsResponse. The pure
// ComputeSessionOptions test coverage lives in composedSession_test.go;
// this file covers the enrichment seam in both budget and count modes.
package terminalTrainer_tests

import (
	"os"
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

// withBudgetFlag temporarily sets OCF_FEATURE_BUDGET_QUOTAS to "1" and
// restores the previous value on cleanup.
func withBudgetFlag(t *testing.T, on bool) {
	t.Helper()
	saved, hadValue := os.LookupEnv("OCF_FEATURE_BUDGET_QUOTAS")
	t.Cleanup(func() {
		if hadValue {
			os.Setenv("OCF_FEATURE_BUDGET_QUOTAS", saved)
		} else {
			os.Unsetenv("OCF_FEATURE_BUDGET_QUOTAS")
		}
	})
	if on {
		os.Setenv("OCF_FEATURE_BUDGET_QUOTAS", "1")
	} else {
		os.Unsetenv("OCF_FEATURE_BUDGET_QUOTAS")
	}
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
	withBudgetFlag(t, true)

	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "BudgetEnrich",
		QuotaModel:  "budget",
		MaxCPU:      8,
		MaxMemoryMB: 4096,
	}

	opts := budgetSessionOptions()
	svc.EnrichSessionOptionsBudget(opts, plan, "u-enrich", nil)

	// With 8 vCPU / 4096 MiB and zero usage:
	//   xs (1c/256MiB)  → min(8/1, 4096/256)  = 8
	//   s  (1c/512MiB)  → min(8/1, 4096/512)  = 8
	//   m  (2c/1024MiB) → min(8/2, 4096/1024) = 4
	//   l  (4c/2048MiB) → min(8/4, 4096/2048) = 2
	//   xl (4c/4096MiB) → min(8/4, 4096/4096) = 1
	assert.Equal(t, 8, findSize(t, opts, "XS").RemainingCount)
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
	withBudgetFlag(t, true)

	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "BudgetEnrichQuota",
		QuotaModel:  "budget",
		MaxCPU:      4,
		MaxMemoryMB: 2048,
	}

	// Seed an M (2c/1g) so usage is non-zero.
	insertExistingTerminal(t, db, "u-quota", nil, "running", "ephemeral", 2, 1024)

	opts := budgetSessionOptions()
	svc.EnrichSessionOptionsBudget(opts, plan, "u-quota", nil)

	require.NotNil(t, opts.Quota, "Quota block must be present in budget mode")
	assert.Equal(t, 4, opts.Quota.MaxCPU)
	assert.Equal(t, 2048, opts.Quota.MaxMemoryMB)
	assert.Equal(t, 2, opts.Quota.UsedCPU)
	assert.Equal(t, 1024, opts.Quota.UsedMemoryMB)
	assert.Equal(t, 2, opts.Quota.RemainingCPU)
	assert.Equal(t, 1024, opts.Quota.RemainingMemoryMB)
	assert.Equal(t, "user", opts.Quota.Scope, "personal context → scope=user")
}

// TestSessionOptions_CountMode_UnlimitedScope — count-mode plans must
// emit Scope="unlimited" so the frontend renders the legacy shape.
func TestSessionOptions_CountMode_UnlimitedScope(t *testing.T) {
	withBudgetFlag(t, true) // flag on, but plan is count-mode

	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Count",
		QuotaModel:             "count",
		MaxConcurrentTerminals: 5,
	}

	opts := budgetSessionOptions()
	svc.EnrichSessionOptionsBudget(opts, plan, "u-count", nil)

	require.NotNil(t, opts.Quota)
	assert.Equal(t, "unlimited", opts.Quota.Scope,
		"count-mode plan must emit Scope=unlimited")
	assert.Equal(t, 0, opts.Quota.MaxCPU)
	assert.Equal(t, 0, opts.Quota.RemainingCPU)

	// Per-size RemainingCount should stay at zero in count-mode.
	for _, s := range opts.AllowedSizes {
		assert.Equal(t, 0, s.RemainingCount,
			"size %s must have zero remaining_count in count-mode", s.Key)
	}

	// But MemoryMB still gets stamped — independent of mode.
	assert.Equal(t, 1024, findSize(t, opts, "M").MemoryMB)
}

// TestSessionOptions_FeatureFlagOff_BypassesBudgetEnrichment — when the
// env var is off, even a budget-mode plan reverts to Scope=unlimited.
func TestSessionOptions_FeatureFlagOff_BypassesBudgetEnrichment(t *testing.T) {
	withBudgetFlag(t, false) // flag explicitly off

	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "BudgetButFlagOff",
		QuotaModel:  "budget",
		MaxCPU:      4,
		MaxMemoryMB: 2048,
	}

	opts := budgetSessionOptions()
	svc.EnrichSessionOptionsBudget(opts, plan, "u-flag-off", nil)

	require.NotNil(t, opts.Quota)
	assert.Equal(t, "unlimited", opts.Quota.Scope,
		"flag off must short-circuit even for budget-mode plans")
	assert.Equal(t, 0, opts.Quota.UsedCPU)
}

// TestSessionOptions_BudgetMode_OrgScope — when orgID is non-nil, Scope
// must be "organization" so dashboards can label the budget accordingly.
func TestSessionOptions_BudgetMode_OrgScope(t *testing.T) {
	withBudgetFlag(t, true)

	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "OrgBudget",
		QuotaModel:  "budget",
		MaxCPU:      8,
		MaxMemoryMB: 4096,
	}
	orgID := uuid.New()

	opts := budgetSessionOptions()
	svc.EnrichSessionOptionsBudget(opts, plan, "u-org", &orgID)

	require.NotNil(t, opts.Quota)
	assert.Equal(t, "organization", opts.Quota.Scope,
		"non-nil orgID → scope=organization")
}
