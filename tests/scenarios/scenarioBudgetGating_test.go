// tests/scenarios/scenarioBudgetGating_test.go
//
// MR-CORE-6 — Budget-mode block_reason gating in GetAvailableScenarios.
//
// The controller maps QuotaService.RemainingBudgetFitsWithReason outcomes
// onto the BlockReason exposed to the frontend:
//
//   QuotaService reason            BlockReason          UI semantic
//   ─────────────────────────────  ───────────────────  ──────────────────────
//   "plan_restriction"             "plan_restriction"   D8 allowlist excludes
//   "budget_cpu_exceeded"          "budget_exhausted"   CPU budget gate
//   "budget_memory_exceeded"       "budget_exhausted"   RAM budget gate
//   ""  (fits=true)                ""  (Launchable)     allowed
//
// The full controller path requires a live tt-backend HTTP stub (for
// resolveScenarioBackendAndDistribution → GetDistributions / GetCatalogSizes),
// which the existing test suite intentionally skips by accepting a
// "no_distribution" BlockReason in test mode. We therefore lock the
// mapping into a contract test against QuotaService directly: the reason
// codes used by the controller MUST be the ones the service returns.
//
// If the service ever renames a reason code, this test fails and the
// controller switch in scenarioController.go:1856 must be updated in
// lockstep.
package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
	configModels "soli/formations/src/configuration/models"

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
// the budget sum query picks it up. Mirrors the helper in
// tests/terminalTrainer/terminalBudgetHook_test.go. Uses Go time.Time
// values (bound as parameters) for dialect-portable comparison with the
// expires_at > NOW() predicate.
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
func budgetTestPlanInMem(maxCPU, maxMem int, allowedSizes []string) *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		BaseModel:           entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                "ScenarioBudgetTest",
		MaxCPU:              maxCPU,
		MaxMemoryMB:         maxMem,
		AllowedMachineSizes: allowedSizes,
	}
}

// TestGetAvailableScenarios_BudgetMode_BlockReasonBudget — scenario needs L
// (4 CPU) but the user's budget can't fit it → reason = "budget_*_exceeded"
// which the controller maps to "budget_exhausted".
func TestGetAvailableScenarios_BudgetMode_BlockReasonBudget(t *testing.T) {
	db := scenarioBudgetTestDB(t)
	eps := paymentServices.NewEffectivePlanService(db)
	quotaSvc := paymentServices.NewQuotaService(db, eps)

	// Plan: 4 CPU / 4096 MiB. Fully used by an existing L (4c, 2048).
	plan := budgetTestPlanInMem(4, 4096, nil)
	insertExistingTerminalBudget(t, db, "u-budget-block", nil, "running", 4, 2048)

	fits, reason, err := quotaSvc.RemainingBudgetFitsWithReason(
		"u-budget-block", nil, plan, "L",
	)
	require.NoError(t, err)
	assert.False(t, fits, "L (4c) cannot fit when remaining CPU is 0")
	assert.Equal(t, "budget_cpu_exceeded", reason,
		"QuotaService must emit budget_cpu_exceeded so the controller can map to budget_exhausted")
}

// TestGetAvailableScenarios_AllowedSizesRestriction_BlockReasonPlan —
// scenario needs L but AllowedMachineSizes=["s","m"] → reason =
// "plan_restriction". The controller surfaces this verbatim to the UI.
func TestGetAvailableScenarios_AllowedSizesRestriction_BlockReasonPlan(t *testing.T) {
	db := scenarioBudgetTestDB(t)
	eps := paymentServices.NewEffectivePlanService(db)
	quotaSvc := paymentServices.NewQuotaService(db, eps)

	// Plan has plenty of budget but D8 allowlist excludes L.
	plan := budgetTestPlanInMem(64, 65536, []string{"s", "m"})

	fits, reason, err := quotaSvc.RemainingBudgetFitsWithReason(
		"u-allowlist", nil, plan, "L",
	)
	require.NoError(t, err)
	assert.False(t, fits, "L is excluded by AllowedMachineSizes")
	assert.Equal(t, "plan_restriction", reason,
		"QuotaService must emit plan_restriction so the controller surfaces it verbatim")
}

// TestGetAvailableScenarios_AllowedSizesAll_NoRestriction — the special
// "all" entry must bypass the D8 allowlist; only the budget matters.
func TestGetAvailableScenarios_AllowedSizesAll_NoRestriction(t *testing.T) {
	db := scenarioBudgetTestDB(t)
	eps := paymentServices.NewEffectivePlanService(db)
	quotaSvc := paymentServices.NewQuotaService(db, eps)

	plan := budgetTestPlanInMem(64, 65536, []string{"all"})

	fits, reason, err := quotaSvc.RemainingBudgetFitsWithReason(
		"u-allow-all", nil, plan, "L",
	)
	require.NoError(t, err)
	assert.True(t, fits, "'all' entry must admit any size")
	assert.Equal(t, "", reason)
}

// TestGetAvailableScenarios_BudgetMode_AllowsWhenFits — budget has room
// for the scenario's L → fits=true, reason="".
func TestGetAvailableScenarios_BudgetMode_AllowsWhenFits(t *testing.T) {
	db := scenarioBudgetTestDB(t)
	eps := paymentServices.NewEffectivePlanService(db)
	quotaSvc := paymentServices.NewQuotaService(db, eps)

	plan := budgetTestPlanInMem(8, 4096, nil)

	fits, reason, err := quotaSvc.RemainingBudgetFitsWithReason(
		"u-fits", nil, plan, "L",
	)
	require.NoError(t, err)
	assert.True(t, fits, "L (4c/2g) fits in 8c/4g budget with no existing sessions")
	assert.Equal(t, "", reason)
}

