// tests/terminalTrainer/composedSessionBudget_test.go
//
// MR-CORE-6 — Budget enforcement in StartComposedSession.
//
// The TerminalBudgetHook (MR-CORE-5) is a no-op on the StartComposedSession
// path because that flow persists the Terminal directly via
// repository.CreateTerminalSession, bypassing the generic Create flow that
// fires hooks. These tests exercise the explicit EnforceBudget seam
// (services.EnforceBudget) added by MR-CORE-6 — the same gate
// StartComposedSession runs inline before the Terminal is persisted.
//
// The full HTTP path (StartComposedSession → tt-backend POST /sessions) is
// out of scope for unit tests because it requires a fake tt-backend.
// Instead we drive the budget seam directly with the real QuotaService over
// SQLite, which is exactly what the production path does post-validation.
package terminalTrainer_tests

import (
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "soli/formations/src/configuration"
	entityManagementModels "soli/formations/src/entityManagement/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/terminalTrainer/services"
)

// budgetTestPlan builds an in-memory budget-mode plan.
func budgetTestPlan(maxCPU, maxMem int) *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "BudgetTest",
		QuotaModel:  "budget",
		MaxCPU:      maxCPU,
		MaxMemoryMB: maxMem,
	}
}

// countTestPlan builds an in-memory legacy count-mode plan.
func countTestPlan(maxConcurrent int) *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "CountTest",
		QuotaModel:             "count",
		MaxConcurrentTerminals: maxConcurrent,
	}
}

// TestStartComposedSession_BudgetMode_RejectsOverBudget — plan MaxCPU=4,
// used=4 (one L) → request L → BudgetRejection with budget_cpu_exceeded.
func TestStartComposedSession_BudgetMode_RejectsOverBudget(t *testing.T) {
	db := freshTestDB(t)
	eps := paymentServices.NewEffectivePlanService(db)
	quotaSvc := paymentServices.NewQuotaService(db, eps)

	plan := budgetTestPlan(4, 8192)
	user := "u-overbudget"

	// Seed an existing L (4 CPU, 2048 MiB) running, expiring in 1h. This
	// fully consumes the CPU budget so the next L is rejected.
	insertExistingTerminal(t, db, user, nil, "running", "ephemeral", 4, 2048)

	// Request another L → CPU axis exhausted.
	err := services.EnforceBudget(quotaSvc, user, nil, plan, 4, 2048)
	require.Error(t, err)

	var rej *services.BudgetRejection
	require.True(t, errors.As(err, &rej), "expected *services.BudgetRejection, got %T", err)
	assert.Equal(t, "budget_cpu_exceeded", rej.Reason)
	assert.Equal(t, 0, rej.RemainingCPU,
		"CPU axis already at the cap — RemainingCPU should clamp to 0")
}

// TestStartComposedSession_BudgetMode_AllowsWithinBudget — plan MaxCPU=8,
// used=0 → request M (2c/1g) → allowed.
func TestStartComposedSession_BudgetMode_AllowsWithinBudget(t *testing.T) {
	db := freshTestDB(t)
	eps := paymentServices.NewEffectivePlanService(db)
	quotaSvc := paymentServices.NewQuotaService(db, eps)

	plan := budgetTestPlan(8, 4096)

	err := services.EnforceBudget(quotaSvc, "u-within-budget", nil, plan, 2, 1024)
	require.NoError(t, err, "M (2c/1g) fits in 8c/4g budget with no existing sessions")
}

// TestStartComposedSession_BudgetMode_RejectsOverBudgetMemory — RAM axis.
func TestStartComposedSession_BudgetMode_RejectsOverBudgetMemory(t *testing.T) {
	db := freshTestDB(t)
	eps := paymentServices.NewEffectivePlanService(db)
	quotaSvc := paymentServices.NewQuotaService(db, eps)

	plan := budgetTestPlan(16, 2048) // CPU-abundant, RAM-bound
	user := "u-mem-over"

	// Seed one L (4c, 2048 MiB) → RAM fully spent.
	insertExistingTerminal(t, db, user, nil, "running", "ephemeral", 4, 2048)

	// Request another L → RAM axis rejection.
	err := services.EnforceBudget(quotaSvc, user, nil, plan, 4, 2048)
	require.Error(t, err)

	var rej *services.BudgetRejection
	require.True(t, errors.As(err, &rej))
	assert.Equal(t, "budget_memory_exceeded", rej.Reason)
}

// TestStartComposedSession_CountMode_BypassesBudgetCheck — count-mode plans
// short-circuit even with heavy usage; the slot middleware enforces.
func TestStartComposedSession_CountMode_BypassesBudgetCheck(t *testing.T) {
	db := freshTestDB(t)
	eps := paymentServices.NewEffectivePlanService(db)
	quotaSvc := paymentServices.NewQuotaService(db, eps)

	plan := countTestPlan(1)
	user := "u-count"

	// Seed a heavy footprint that would blow any budget — count-mode must
	// ignore it.
	insertExistingTerminal(t, db, user, nil, "running", "ephemeral", 4, 4096)

	err := services.EnforceBudget(quotaSvc, user, nil, plan, 4, 4096)
	require.NoError(t, err, "count-mode plan must short-circuit budget enforcement")
}

// TestStartComposedSession_NilQuotaService_FailsOpen — defensive: if a test
// builds the service without QuotaService wired, the explicit check is a
// no-op (the hook still gates the generic-create path).
func TestStartComposedSession_NilQuotaService_FailsOpen(t *testing.T) {
	plan := budgetTestPlan(1, 256)
	err := services.EnforceBudget(nil, "u-no-quota", nil, plan, 8, 8192)
	require.NoError(t, err, "nil QuotaService must fail open (defense-in-depth on the hook)")
}

// TestStartComposedSession_BudgetMode_OrgScopedQuota — when orgID is provided,
// the budget sum joins through organization_members; another org's usage must
// not bleed in.
func TestStartComposedSession_BudgetMode_OrgScopedQuota(t *testing.T) {
	db := freshTestDB(t)
	eps := paymentServices.NewEffectivePlanService(db)
	quotaSvc := paymentServices.NewQuotaService(db, eps)

	plan := budgetTestPlan(4, 4096)

	// Org A: owner1, plus member1 owns a running L. That consumes the
	// org-scoped budget.
	orgA := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, orgA.ID, "owner1", orgModels.OrgRoleOwner)
	createTestOrgMember(t, db, orgA.ID, "member1", orgModels.OrgRoleMember)
	insertExistingTerminal(t, db, "member1", &orgA.ID, "running", "ephemeral", 4, 2048)

	// Org B: independent. Owner2 has no usage.
	orgB := createTestOrgForHistory(t, db, "owner2")
	createTestOrgMember(t, db, orgB.ID, "owner2", orgModels.OrgRoleOwner)

	// owner1 in org A: budget exhausted on CPU → reject any new L.
	errA := services.EnforceBudget(quotaSvc, "owner1", &orgA.ID, plan, 4, 2048)
	require.Error(t, errA, "org A budget should be exhausted")
	var rejA *services.BudgetRejection
	require.True(t, errors.As(errA, &rejA))
	assert.Equal(t, "budget_cpu_exceeded", rejA.Reason)

	// owner2 in org B: nothing consumed → same L fits.
	errB := services.EnforceBudget(quotaSvc, "owner2", &orgB.ID, plan, 4, 2048)
	require.NoError(t, errB, "org B has zero usage; the same request must fit")
}

// TestIsBudgetQuotasEnabled_DefaultsOff guards the feature-flag default.
// Setting the env var to "1" must flip it on for this process.
func TestIsBudgetQuotasEnabled_DefaultsOff(t *testing.T) {
	// Save + restore the env var to keep test isolation.
	saved, hadValue := os.LookupEnv("OCF_FEATURE_BUDGET_QUOTAS")
	t.Cleanup(func() {
		if hadValue {
			os.Setenv("OCF_FEATURE_BUDGET_QUOTAS", saved)
		} else {
			os.Unsetenv("OCF_FEATURE_BUDGET_QUOTAS")
		}
	})

	os.Unsetenv("OCF_FEATURE_BUDGET_QUOTAS")
	assert.False(t, config.IsBudgetQuotasEnabled(),
		"flag must default to off")

	os.Setenv("OCF_FEATURE_BUDGET_QUOTAS", "1")
	assert.True(t, config.IsBudgetQuotasEnabled(),
		"OCF_FEATURE_BUDGET_QUOTAS=1 must flip it on")

	os.Setenv("OCF_FEATURE_BUDGET_QUOTAS", "0")
	assert.False(t, config.IsBudgetQuotasEnabled(),
		"explicit 0 must keep it off")
}
