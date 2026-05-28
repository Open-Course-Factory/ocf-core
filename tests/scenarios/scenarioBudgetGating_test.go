// tests/scenarios/scenarioBudgetGating_test.go
//
// Budget block_reason gating in GetAvailableScenarios.
//
// The controller maps QuotaService.RemainingBudgetFits outcomes onto the
// BlockReason exposed to the frontend: when the size doesn't fit in the
// remaining budget, BlockReason = "budget_exhausted".
//
// The full controller path requires a live tt-backend HTTP stub (for
// resolveScenarioBackendAndDistribution → GetDistributions /
// GetCatalogSizes), which the existing test suite intentionally skips by
// accepting a "no_distribution" BlockReason in test mode. We therefore
// lock the contract by exercising QuotaService directly.
package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	configModels "soli/formations/src/configuration/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// scenarioBudgetTestDB returns a fresh SQLite DB with the tables needed for
// these tests. Lives here (rather than the package-level main_test.go) so
// the suite stays self-contained.
func scenarioBudgetTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	err = db.AutoMigrate(
		&terminalModels.Terminal{},
		&terminalModels.UserTerminalKey{},
		&orgModels.Organization{},
		&orgModels.OrganizationMember{},
		&paymentModels.SubscriptionPlan{},
		&paymentModels.OrganizationSubscription{},
		&paymentModels.UserSubscription{},
		&paymentModels.UsageMetrics{},
		&configModels.Feature{},
	)
	require.NoError(t, err)
	return db
}

// insertExistingTerminalBudget seeds a Terminal row directly via SQL so
// the budget sum query picks it up.
func insertExistingTerminalBudget(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID, state string, cpu, memMB int) {
	t.Helper()
	id := uuid.New().String()
	var orgVal any
	if orgID != nil {
		orgVal = orgID.String()
	}
	now := time.Now()
	expires := now.Add(time.Hour)
	require.NoError(t, db.Exec(`INSERT INTO terminals
		(id, created_at, updated_at, user_id, organization_id, session_id, state, persistence_mode,
		 size_cpu, size_memory_mb, expires_at, machine_size, user_terminal_key_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'ephemeral', ?, ?, ?, '', ?)`,
		id, now, now, userID, orgVal, "sess-"+id, state, cpu, memMB, expires, uuid.New().String()).Error)
}

// budgetTestPlanInMem builds an in-memory plan with the given CPU/RAM caps.
func budgetTestPlanInMem(maxCPU, maxMem int) *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "ScenarioBudgetTest",
		MaxCPU:      maxCPU,
		MaxMemoryMB: maxMem,
	}
}

// TestGetAvailableScenarios_BudgetMode_BlockReasonBudget — scenario needs L
// (4 CPU) but the user's budget can't fit it → RemainingBudgetFits returns
// false, which the controller maps to BlockReason="budget_exhausted".
func TestGetAvailableScenarios_BudgetMode_BlockReasonBudget(t *testing.T) {
	db := scenarioBudgetTestDB(t)
	eps := paymentServices.NewEffectivePlanService(db)
	quotaSvc := paymentServices.NewQuotaService(db, eps)

	// Plan: 4 CPU / 4096 MiB. Fully used by an existing L (4c, 2048).
	plan := budgetTestPlanInMem(4, 4096)
	insertExistingTerminalBudget(t, db, "u-budget-block", nil, "running", 4, 2048)

	fits, err := quotaSvc.RemainingBudgetFits("u-budget-block", nil, plan, "L")
	require.NoError(t, err)
	assert.False(t, fits, "L (4c) cannot fit when remaining CPU is 0")
}

// TestGetAvailableScenarios_BudgetMode_AllowsWhenFits — budget has room
// for the scenario's L → fits=true.
func TestGetAvailableScenarios_BudgetMode_AllowsWhenFits(t *testing.T) {
	db := scenarioBudgetTestDB(t)
	eps := paymentServices.NewEffectivePlanService(db)
	quotaSvc := paymentServices.NewQuotaService(db, eps)

	plan := budgetTestPlanInMem(8, 4096)

	fits, err := quotaSvc.RemainingBudgetFits("u-fits", nil, plan, "L")
	require.NoError(t, err)
	assert.True(t, fits, "L (4c/2g) fits in 8c/4g budget with no existing sessions")
}
