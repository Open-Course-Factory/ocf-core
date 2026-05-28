// tests/terminalTrainer/terminalBudgetHook_test.go
//
// Tests for TerminalBudgetHook (MR-CORE-5).
//
// The hook is the write-time race-safe complement to QuotaService.CheckBudget
// (MR-CORE-4). It performs three jobs:
//
//  1. Denormalises the size catalog footprint onto the Terminal row
//     (SizeCPU / SizeMemoryMB) — always, even for legacy "count" plans.
//  2. Enforces CPU / RAM budget caps from the resolved plan.
//
// SQLite covers correctness (no real concurrency); a PostgreSQL-only race
// test is gated with `testing.Short()` and a runtime PG check.
package terminalTrainer_tests

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/entityManagement/hooks"
	organizationModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	terminalHooks "soli/formations/src/terminalTrainer/hooks"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// ---------------------------------------------------------------------------
// Stub EffectivePlanService that returns a pre-resolved plan.
//
// We don't want to drive the hook through the full plan-resolution chain
// (UserSubscription, OrganizationSubscription, fallback rules) — that's
// already covered by tests/payment. Here we focus on the hook's logic
// once a plan has been resolved.
// ---------------------------------------------------------------------------

type stubEffectivePlanService struct {
	personalPlan *paymentModels.SubscriptionPlan
	orgPlan      *paymentModels.SubscriptionPlan
	failResolve  bool
}

// GetUserEffectivePlan matches the consolidated interface (MR !239):
// orgID != nil returns the org plan when present, falling back to the
// personal plan; orgID == nil returns the personal plan only.
func (s *stubEffectivePlanService) GetUserEffectivePlan(userID string, orgID *uuid.UUID) (*paymentServices.EffectivePlanResult, error) {
	if s.failResolve {
		return nil, errors.New("plan resolution failure")
	}
	if orgID != nil && s.orgPlan != nil {
		return &paymentServices.EffectivePlanResult{
			Plan:   s.orgPlan,
			Source: paymentServices.PlanSourceOrganization,
			OrganizationSubscription: &paymentModels.OrganizationSubscription{
				OrganizationID: *orgID,
			},
		}, nil
	}
	if s.personalPlan != nil {
		return &paymentServices.EffectivePlanResult{
			Plan:   s.personalPlan,
			Source: paymentServices.PlanSourcePersonal,
		}, nil
	}
	return nil, nil
}

func (s *stubEffectivePlanService) CheckEffectiveUsageLimit(userID string, orgID *uuid.UUID, metricType string, increment int64) (*paymentServices.UsageLimitCheck, error) {
	return nil, errors.New("not implemented in stub")
}

func (s *stubEffectivePlanService) CheckEffectiveUsageLimitFromResult(result *paymentServices.EffectivePlanResult, userID string, metricType string, increment int64) (*paymentServices.UsageLimitCheck, error) {
	return nil, errors.New("not implemented in stub")
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// budgetPlanInMem builds an in-memory plan (not persisted) for hook tests.
// MaxCPU/MaxMemoryMB of 0 means "unlimited" per the contract. The
// trailing []string parameter is retained for call-site compatibility
// (it used to carry AllowedMachineSizes); it is now ignored.
func budgetPlanInMem(name string, maxCPU, maxMem int, _ []string) *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        name,
		MaxCPU:      maxCPU,
		MaxMemoryMB: maxMem,
	}
}

// insertExistingTerminal writes a Terminal row with the exact state/persistence
// + size footprint the budget query reads from. Uses raw SQL to bypass the
// BeforeCreate hook (which is the system under test).
func insertExistingTerminal(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID, state, persistence string, cpu, memMB int) {
	t.Helper()
	id := uuid.New().String()
	var orgVal any
	if orgID != nil {
		orgVal = orgID.String()
	} else {
		orgVal = nil
	}
	err := db.Exec(`INSERT INTO terminals
		(id, created_at, updated_at, user_id, organization_id, session_id, state, persistence_mode, size_cpu, size_memory_mb, expires_at, machine_size, user_terminal_key_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
		id, time.Now(), time.Now(), userID, orgVal,
		"sess-"+id, state, persistence, cpu, memMB, time.Now().Add(time.Hour), uuid.New().String()).Error
	require.NoError(t, err)
}

// insertExistingTerminalWithExpiry mirrors insertExistingTerminal but lets
// the caller pin expires_at. Used by past-expiry tests to verify the
// `expires_at > NOW()` predicate excludes zombie rows from the locked sum
// (mirrors OccupiesSlotScope's zombie-exclusion rule).
func insertExistingTerminalWithExpiry(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID, state, persistence string, cpu, memMB int, expiresAt time.Time) {
	t.Helper()
	id := uuid.New().String()
	var orgVal any
	if orgID != nil {
		orgVal = orgID.String()
	} else {
		orgVal = nil
	}
	err := db.Exec(`INSERT INTO terminals
		(id, created_at, updated_at, user_id, organization_id, session_id, state, persistence_mode, size_cpu, size_memory_mb, expires_at, machine_size, user_terminal_key_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
		id, time.Now(), time.Now(), userID, orgVal,
		"sess-"+id, state, persistence, cpu, memMB, expiresAt, uuid.New().String()).Error
	require.NoError(t, err)
}

// newHookForTest constructs the hook with a stub EffectivePlanService
// preloaded with the given plan(s).
func newHookForTest(db *gorm.DB, personal, org *paymentModels.SubscriptionPlan) hooks.Hook {
	return terminalHooks.NewTerminalBudgetHook(db, &stubEffectivePlanService{
		personalPlan: personal,
		orgPlan:      org,
	})
}

// execBeforeCreate runs the hook's Execute on a *Terminal with the given
// user / org / size — mirroring how genericService fires it.
func execBeforeCreate(hook hooks.Hook, terminal *terminalModels.Terminal) error {
	return hook.Execute(&hooks.HookContext{
		EntityName: "Terminal",
		HookType:   hooks.BeforeCreate,
		NewEntity:  terminal,
		UserID:     terminal.UserID,
	})
}

// ---------------------------------------------------------------------------
// 1) Size denormalisation
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_PopulatesSizeFields(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("Pro", 8, 4096, nil)
	hook := newHookForTest(db, plan, nil)

	terminal := &terminalModels.Terminal{
		UserID:      "u-populates",
		MachineSize: "m", // catalog: cpu=2, mem=1024
	}

	err := execBeforeCreate(hook, terminal)
	require.NoError(t, err)

	assert.Equal(t, 2, terminal.SizeCPU, "M size cpu=2 must be snapshot onto Terminal")
	assert.Equal(t, 1024, terminal.SizeMemoryMB, "M size memory_mb=1024 must be snapshot onto Terminal")
}

// ---------------------------------------------------------------------------
// 2) Allowed within budget
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_AllowedWithinBudget(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("Pro", 8, 4096, nil)
	hook := newHookForTest(db, plan, nil)

	terminal := &terminalModels.Terminal{
		UserID:      "u-within-budget",
		MachineSize: "M",
	}

	err := execBeforeCreate(hook, terminal)
	require.NoError(t, err, "M (2c/1g) fits in 8c/4g budget with no existing sessions")
}

// ---------------------------------------------------------------------------
// 3) Rejected: CPU axis
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_RejectedOverBudget_CPU(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("Tight", 4, 8192, nil) // 4c / 8g
	hook := newHookForTest(db, plan, nil)

	user := "u-overbudget-cpu"
	// Pre-existing L (4 CPU) running → budget fully used on CPU axis.
	insertExistingTerminal(t, db, user, nil, "running", "ephemeral", 4, 2048)

	terminal := &terminalModels.Terminal{
		UserID:      user,
		MachineSize: "L", // requires 4 more CPU → reject
	}

	err := execBeforeCreate(hook, terminal)
	require.Error(t, err)
	var budgetErr *terminalHooks.ErrBudgetExhausted
	require.ErrorAs(t, err, &budgetErr)
	assert.Equal(t, terminalHooks.BudgetAxisCPU, budgetErr.Axis)
	assert.Equal(t, 4, budgetErr.Limit)
	assert.Equal(t, 4, budgetErr.Current)
	assert.Equal(t, 4, budgetErr.Requested)
}

// ---------------------------------------------------------------------------
// 4) Rejected: memory axis
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_RejectedOverBudget_Memory(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("RAMBound", 16, 2048, nil) // 16c / 2g
	hook := newHookForTest(db, plan, nil)

	user := "u-overbudget-mem"
	// Pre-existing L (2 GiB used) running. 2g of RAM fully consumed.
	insertExistingTerminal(t, db, user, nil, "running", "ephemeral", 4, 2048)

	terminal := &terminalModels.Terminal{
		UserID:      user,
		MachineSize: "L", // wants 2 GiB more → reject
	}

	err := execBeforeCreate(hook, terminal)
	require.Error(t, err)
	var budgetErr *terminalHooks.ErrBudgetExhausted
	require.ErrorAs(t, err, &budgetErr)
	assert.Equal(t, terminalHooks.BudgetAxisMemory, budgetErr.Axis)
}

// ---------------------------------------------------------------------------
// 6) Persistent stopped sessions count against the budget
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_PersistentStoppedCounts(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("Tight", 2, 1024, nil) // 2c / 1g — exactly one M
	hook := newHookForTest(db, plan, nil)

	user := "u-persistent-stopped"
	// One persistent + stopped M-size terminal already exists. By D6 it
	// counts: full budget consumed.
	insertExistingTerminal(t, db, user, nil, "stopped", "persistent", 2, 1024)

	terminal := &terminalModels.Terminal{
		UserID:      user,
		MachineSize: "M",
	}

	err := execBeforeCreate(hook, terminal)
	require.Error(t, err, "persistent stopped session must count → second M is rejected")
	var budgetErr *terminalHooks.ErrBudgetExhausted
	require.ErrorAs(t, err, &budgetErr)
}

// ---------------------------------------------------------------------------
// 7) Stopped ephemeral sessions DO count (D6', supersedes D6)
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_EphemeralStoppedAlsoCounts(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("Tight", 2, 1024, nil)
	hook := newHookForTest(db, plan, nil)

	user := "u-ephemeral-stopped"
	// One ephemeral + stopped M-size terminal. Under D6' (supersedes D6),
	// "a stop is a stop": it MUST count against the budget until sync
	// confirms tt-backend reaped the container.
	insertExistingTerminal(t, db, user, nil, "stopped", "ephemeral", 2, 1024)

	terminal := &terminalModels.Terminal{
		UserID:      user,
		MachineSize: "M",
	}

	err := execBeforeCreate(hook, terminal)
	require.Error(t, err, "stopped ephemeral must count → new M is rejected (D6')")
	var budgetErr *terminalHooks.ErrBudgetExhausted
	require.ErrorAs(t, err, &budgetErr)
}

// Past-expiry zombies must not count against the locked budget sum either.
// Mirrors the `expires_at > NOW()` clause that MR !239 added to
// OccupiesSlotScope: a row whose proxy session is long gone but whose
// state column was never reset must not block a new session. Without the
// filter, the budget check would see 2c/1g consumed and reject the new
// M-size request; with the filter, the zombie is excluded and the
// request fits.
func TestTerminalBudgetHook_BeforeCreate_PastExpirySessionsDoNotCount(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("Tight", 2, 1024, nil)
	hook := newHookForTest(db, plan, nil)

	user := "u-past-expiry"
	// A persistent + running M-size terminal whose expires_at is in the past.
	// Lifecycle (D6) says it should count, but past-expiry excludes it.
	pastExpiry := time.Now().Add(-1 * time.Hour)
	insertExistingTerminalWithExpiry(t, db, user, nil, "running", "persistent", 2, 1024, pastExpiry)

	terminal := &terminalModels.Terminal{
		UserID:      user,
		MachineSize: "M",
	}

	err := execBeforeCreate(hook, terminal)
	require.NoError(t, err, "past-expiry zombie session must NOT count → new M fits")
}

// ---------------------------------------------------------------------------
// 8) Org-scoped: counts across all org members
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_OrgScoped(t *testing.T) {
	db := freshTestDB(t)

	orgID := uuid.New()
	// Create the org and three members so the org-scoped sum sees all three.
	require.NoError(t, db.Omit("Metadata").Create(&organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: orgID},
		Name:             "team-budget",
		DisplayName:      "Team Budget",
		OwnerUserID:      "u-org-a",
		OrganizationType: organizationModels.OrgTypeTeam,
	}).Error)
	for _, uid := range []string{"u-org-a", "u-org-b", "u-org-c", "u-org-d"} {
		require.NoError(t, db.Omit("Metadata").Create(&organizationModels.OrganizationMember{
			BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID: orgID,
			UserID:         uid,
			Role:           "member",
			JoinedAt:       time.Now(),
			IsActive:       true,
		}).Error)
	}

	plan := budgetPlanInMem("Team", 3, 4096, nil) // 3 CPU total
	hook := newHookForTest(db, nil, plan)

	// 3 members each have a running S (1 CPU). Total 3/3 CPU used.
	insertExistingTerminal(t, db, "u-org-a", &orgID, "running", "ephemeral", 1, 512)
	insertExistingTerminal(t, db, "u-org-b", &orgID, "running", "ephemeral", 1, 512)
	insertExistingTerminal(t, db, "u-org-c", &orgID, "running", "ephemeral", 1, 512)

	// 4th member requests another S → must be rejected.
	terminal := &terminalModels.Terminal{
		UserID:         "u-org-d",
		OrganizationID: &orgID,
		MachineSize:    "S",
	}

	err := execBeforeCreate(hook, terminal)
	require.Error(t, err, "org-wide sum (3 CPU) exhausts the 3-CPU team budget")
	var budgetErr *terminalHooks.ErrBudgetExhausted
	require.ErrorAs(t, err, &budgetErr)
	assert.Equal(t, terminalHooks.BudgetAxisCPU, budgetErr.Axis)
}

// ---------------------------------------------------------------------------
// Unknown size fails closed
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_UnknownSize_Error(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("Pro", 8, 4096, nil)
	hook := newHookForTest(db, plan, nil)

	terminal := &terminalModels.Terminal{
		UserID:      "u-unknown-size",
		MachineSize: "potato",
	}

	err := execBeforeCreate(hook, terminal)
	require.Error(t, err)
	var unkErr *terminalHooks.ErrUnknownMachineSize
	require.ErrorAs(t, err, &unkErr)
	assert.Equal(t, "potato", unkErr.Requested)
}

// ---------------------------------------------------------------------------
// 11) Unlimited budget allows any size
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_UnlimitedBudget(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("Unlimited", 0, 0, nil) // 0/0 = unlimited
	hook := newHookForTest(db, plan, nil)

	terminal := &terminalModels.Terminal{
		UserID:      "u-unlimited",
		MachineSize: "XL", // 4c / 4g
	}

	err := execBeforeCreate(hook, terminal)
	require.NoError(t, err, "0-cap MaxCPU/MaxMemoryMB → unlimited → any size allowed")

	assert.Equal(t, 4, terminal.SizeCPU)
	assert.Equal(t, 4096, terminal.SizeMemoryMB)
}

// ---------------------------------------------------------------------------
// 12) Race condition (PostgreSQL only — gated by testing.Short() + env)
// ---------------------------------------------------------------------------
//
// This test asserts the BeforeCreate hook serialises concurrent session
// starts via SELECT ... FOR UPDATE. SQLite is single-writer by default
// so the test would pass trivially without proving anything; we skip it
// when not running under PostgreSQL.
//
// To run locally: TEST_PG_DSN="postgres://..." go test -run RaceCondition

func TestTerminalBudgetHook_RaceCondition_ConcurrentStarts(t *testing.T) {
	if testing.Short() {
		t.Skip("race-condition test requires PostgreSQL; skipped with -short")
	}
	// We don't spin a PG instance here — the test reads sharedTestDB
	// which is SQLite. We document the limitation and skip rather than
	// silently pass.
	if sharedTestDB == nil || sharedTestDB.Dialector == nil || sharedTestDB.Dialector.Name() != "postgres" {
		t.Skip("race-condition test requires PostgreSQL; current driver = " +
			sharedTestDBDriverName() + ". Run with TEST_PG_DSN set in a PG-backed CI job.")
	}

	db := freshTestDB(t)
	plan := budgetPlanInMem("Race", 4, 2048, nil) // exactly one L fits
	hook := newHookForTest(db, plan, nil)

	user := "u-race"
	const goroutines = 5
	var wg sync.WaitGroup
	results := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			terminal := &terminalModels.Terminal{
				UserID:      user,
				MachineSize: "L",
			}
			err := execBeforeCreate(hook, terminal)
			// On success we also insert (mimics what genericService does
			// after the BeforeCreate hook returns nil).
			if err == nil {
				insertExistingTerminal(t, db, user, nil, "running", "ephemeral", 4, 2048)
			}
			results[idx] = err
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, e := range results {
		if e == nil {
			successes++
		}
	}
	assert.Equal(t, 1, successes, "exactly one goroutine must succeed; the rest must hit budget_exhausted")
}

// sharedTestDBDriverName reports the active driver for diagnostics.
func sharedTestDBDriverName() string {
	if sharedTestDB == nil || sharedTestDB.Dialector == nil {
		return "<nil>"
	}
	return sharedTestDB.Dialector.Name()
}
