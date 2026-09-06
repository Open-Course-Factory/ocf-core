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

	// globalResolvesToOrg models resolveGlobal returning an ORGANIZATION's plan
	// for a request that carried no org context — the real behaviour, since
	// resolveGlobal picks the highest-priority plan the user holds anywhere.
	// That combination is the whole point of #457: the plan is the org's, so the
	// budget must be the org's, even though the request named no organization.
	globalResolvesToOrg *uuid.UUID
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
			// Mirrors the real resolver: an organization's plan draws on that
			// organization's pool. This is what the budget gate scopes by (#457),
			// so a stub omitting it would silently exercise personal counting.
			ScopeOrganizationID: orgID,
		}, nil
	}
	if orgID == nil && s.globalResolvesToOrg != nil && s.orgPlan != nil {
		return &paymentServices.EffectivePlanResult{
			Plan:                s.orgPlan,
			Source:              paymentServices.PlanSourceOrganization,
			ScopeOrganizationID: s.globalResolvesToOrg,
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

// CanRunClassrooms resolves through this stub's own GetUserEffectivePlan so the
// verdict tracks whatever plan the test set up, rather than being a second,
// independently-stubbed answer — the exact split that made this predicate drift
// in production (#453).
// CanPurchaseSeats mirrors the real service's narrower resolution: the plan the
// user holds themselves. This stub has no notion of inheritance, so the personal
// plan is that plan.
func (s *stubEffectivePlanService) CanPurchaseSeats(userID string) paymentServices.ClassroomEntitlement {
	return paymentServices.ClassroomEntitlementFor(s.personalPlan)
}

// GetUserBudgetCeiling reports the stub's personal plan as the user's ceiling.
// These tests exercise the budget hook, not key provisioning, so the simplest
// faithful answer is enough to satisfy the interface.
func (s *stubEffectivePlanService) GetUserBudgetCeiling(userID string) (paymentServices.UserBudgetCeiling, error) {
	if s.personalPlan == nil {
		return paymentServices.UserBudgetCeiling{}, nil
	}
	return paymentServices.UserBudgetCeiling{
		MaxCPU:         s.personalPlan.MaxCPU,
		MaxMemoryMB:    s.personalPlan.MaxMemoryMB,
		HasEntitlement: true,
	}, nil
}

func (s *stubEffectivePlanService) CanRunClassrooms(userID string, orgID *uuid.UUID) paymentServices.ClassroomEntitlement {
	result, err := s.GetUserEffectivePlan(userID, orgID)
	if err != nil || result == nil {
		return paymentServices.ClassroomEntitlement{Reason: paymentServices.ClassroomDeniedNoPlan}
	}
	return paymentServices.ClassroomEntitlementFor(result.Plan)
}

// ClassroomEntitlementInOrg mirrors the real service, minus the organization-shape
// gate: this stub holds no organizations, so there is no shape to refuse on. It
// stays consistent with this stub's CanRunClassrooms, which is the property the
// budget tests rely on.
func (s *stubEffectivePlanService) ClassroomEntitlementInOrg(userID string, orgID uuid.UUID, plan *paymentModels.SubscriptionPlan) paymentServices.ClassroomEntitlement {
	return paymentServices.ClassroomEntitlementFor(plan)
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
//
// maxCPU is in millicores (mCPU): 1000 mCPU = 1 vCPU.
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
	eps := &stubEffectivePlanService{
		personalPlan: personal,
		orgPlan:      org,
	}
	return terminalHooks.NewTerminalBudgetHook(db, eps, paymentServices.NewQuotaService(db, eps))
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
	plan := budgetPlanInMem("Pro", 8000, 4096, nil)
	hook := newHookForTest(db, plan, nil)

	terminal := &terminalModels.Terminal{
		UserID:      "u-populates",
		MachineSize: "m", // catalog: cpu=2000 mCPU, mem=1024
	}

	err := execBeforeCreate(hook, terminal)
	require.NoError(t, err)

	assert.Equal(t, 2000, terminal.SizeCPU, "M size cpu=2000 mCPU must be snapshot onto Terminal")
	assert.Equal(t, 1024, terminal.SizeMemoryMB, "M size memory_mb=1024 must be snapshot onto Terminal")
}

// ---------------------------------------------------------------------------
// 2) Allowed within budget
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_AllowedWithinBudget(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("Pro", 8000, 4096, nil)
	hook := newHookForTest(db, plan, nil)

	terminal := &terminalModels.Terminal{
		UserID:      "u-within-budget",
		MachineSize: "M",
	}

	err := execBeforeCreate(hook, terminal)
	require.NoError(t, err, "M (2000 mCPU/1g) fits in 8000 mCPU/4g budget with no existing sessions")
}

// ---------------------------------------------------------------------------
// 3) Rejected: CPU axis
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_RejectedOverBudget_CPU(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("Tight", 4000, 8192, nil) // 4000 mCPU / 8g
	hook := newHookForTest(db, plan, nil)

	user := "u-overbudget-cpu"
	// Pre-existing L (4000 mCPU) running → budget fully used on CPU axis.
	insertExistingTerminal(t, db, user, nil, "running", "ephemeral", 4000, 2048)

	terminal := &terminalModels.Terminal{
		UserID:      user,
		MachineSize: "L", // requires 4000 more mCPU → reject
	}

	err := execBeforeCreate(hook, terminal)
	require.Error(t, err)
	var budgetErr *terminalHooks.ErrBudgetExhausted
	require.ErrorAs(t, err, &budgetErr)
	assert.Equal(t, terminalHooks.BudgetAxisCPU, budgetErr.Axis)
	assert.Equal(t, 4000, budgetErr.Limit)
	assert.Equal(t, 4000, budgetErr.Current)
	assert.Equal(t, 4000, budgetErr.Requested)
}

// ---------------------------------------------------------------------------
// 4) Rejected: memory axis
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_RejectedOverBudget_Memory(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("RAMBound", 16000, 2048, nil) // 16000 mCPU / 2g
	hook := newHookForTest(db, plan, nil)

	user := "u-overbudget-mem"
	// Pre-existing L (2 GiB used) running. 2g of RAM fully consumed.
	insertExistingTerminal(t, db, user, nil, "running", "ephemeral", 4000, 2048)

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
	plan := budgetPlanInMem("Tight", 2000, 1024, nil) // 2000 mCPU / 1g — exactly one M
	hook := newHookForTest(db, plan, nil)

	user := "u-persistent-stopped"
	// One persistent + stopped M-size terminal already exists. By D6 it
	// counts: full budget consumed.
	insertExistingTerminal(t, db, user, nil, "stopped", "persistent", 2000, 1024)

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
	plan := budgetPlanInMem("Tight", 2000, 1024, nil)
	hook := newHookForTest(db, plan, nil)

	user := "u-ephemeral-stopped"
	// One ephemeral + stopped M-size terminal. Under D6' (supersedes D6),
	// "a stop is a stop": it MUST count against the budget until sync
	// confirms tt-backend reaped the container.
	insertExistingTerminal(t, db, user, nil, "stopped", "ephemeral", 2000, 1024)

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
	plan := budgetPlanInMem("Tight", 2000, 1024, nil)
	hook := newHookForTest(db, plan, nil)

	user := "u-past-expiry"
	// A persistent + running M-size terminal whose expires_at is in the past.
	// Lifecycle (D6) says it should count, but past-expiry excludes it.
	pastExpiry := time.Now().Add(-1 * time.Hour)
	insertExistingTerminalWithExpiry(t, db, user, nil, "running", "persistent", 2000, 1024, pastExpiry)

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

	plan := budgetPlanInMem("Team", 3000, 4096, nil) // 3000 mCPU total
	hook := newHookForTest(db, nil, plan)

	// 3 members each have a running S (1000 mCPU). Total 3000/3000 mCPU used.
	insertExistingTerminal(t, db, "u-org-a", &orgID, "running", "ephemeral", 1000, 512)
	insertExistingTerminal(t, db, "u-org-b", &orgID, "running", "ephemeral", 1000, 512)
	insertExistingTerminal(t, db, "u-org-c", &orgID, "running", "ephemeral", 1000, 512)

	// 4th member requests another S → must be rejected.
	terminal := &terminalModels.Terminal{
		UserID:         "u-org-d",
		OrganizationID: &orgID,
		MachineSize:    "S",
	}

	err := execBeforeCreate(hook, terminal)
	require.Error(t, err, "org-wide sum (3000 mCPU) exhausts the 3000-mCPU team budget")
	var budgetErr *terminalHooks.ErrBudgetExhausted
	require.ErrorAs(t, err, &budgetErr)
	assert.Equal(t, terminalHooks.BudgetAxisCPU, budgetErr.Axis)
}

// The bypass #457 closes: identical to the test above except the request carries
// NO organization_id. The plan still resolves to the organization's — resolveGlobal
// picks the highest-priority plan the user holds anywhere — so the budget is still
// the organization's shared pool and the launch must still be refused.
//
// Before the fix the scope came from terminal.OrganizationID, so a nil there meant
// "count this user alone": every member of a school could hold the school's entire
// budget simultaneously just by omitting one optional parameter.
func TestTerminalBudgetHook_OrgPoolAppliesWithoutOrgContext(t *testing.T) {
	db := freshTestDB(t)

	orgID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: orgID},
		Name:             "school-budget",
		DisplayName:      "School Budget",
		OwnerUserID:      "u-school-a",
		OrganizationType: organizationModels.OrgTypeTeam,
	}).Error)
	for _, uid := range []string{"u-school-a", "u-school-b", "u-school-c", "u-school-d"} {
		require.NoError(t, db.Omit("Metadata").Create(&organizationModels.OrganizationMember{
			BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID: orgID,
			UserID:         uid,
			Role:           "member",
			JoinedAt:       time.Now(),
			IsActive:       true,
		}).Error)
	}

	plan := budgetPlanInMem("School", 3000, 4096, nil)
	eps := &stubEffectivePlanService{orgPlan: plan, globalResolvesToOrg: &orgID}
	hook := terminalHooks.NewTerminalBudgetHook(db, eps, paymentServices.NewQuotaService(db, eps))

	// Three members already hold the whole pool.
	insertExistingTerminal(t, db, "u-school-a", &orgID, "running", "ephemeral", 1000, 512)
	insertExistingTerminal(t, db, "u-school-b", &orgID, "running", "ephemeral", 1000, 512)
	insertExistingTerminal(t, db, "u-school-c", &orgID, "running", "ephemeral", 1000, 512)

	// Fourth member launches WITHOUT org context.
	terminal := &terminalModels.Terminal{
		UserID:         "u-school-d",
		OrganizationID: nil,
		MachineSize:    "S",
	}

	err := execBeforeCreate(hook, terminal)

	require.Error(t, err,
		"omitting organization_id must not convert a shared org pool into a per-member one")
	var budgetErr *terminalHooks.ErrBudgetExhausted
	require.ErrorAs(t, err, &budgetErr)
	assert.Equal(t, terminalHooks.BudgetAxisCPU, budgetErr.Axis)
}

// The trainer case, which must NOT become an org pool: the plan is personal, so
// the budget is counted for that user alone even though they belong to an
// organization. A trainer's team org owns no plan; his learners hold their own
// assigned seats.
func TestTerminalBudgetHook_PersonalPlanStaysPerUserInsideAnOrg(t *testing.T) {
	db := freshTestDB(t)

	orgID := uuid.New()
	require.NoError(t, db.Omit("Metadata").Create(&organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: orgID},
		Name:             "trainer-team",
		DisplayName:      "Trainer Team",
		OwnerUserID:      "u-trainer",
		OrganizationType: organizationModels.OrgTypeTeam,
	}).Error)
	for _, uid := range []string{"u-trainer", "u-learner"} {
		require.NoError(t, db.Omit("Metadata").Create(&organizationModels.OrganizationMember{
			BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID: orgID,
			UserID:         uid,
			Role:           "member",
			JoinedAt:       time.Now(),
			IsActive:       true,
		}).Error)
	}

	plan := budgetPlanInMem("Formateur", 2000, 4096, nil)
	// Personal plan only: no org plan, no global-to-org resolution.
	hook := newHookForTest(db, plan, nil)

	// Another member of the same org is using capacity. It must not count against
	// this user, because they do not share a plan.
	insertExistingTerminal(t, db, "u-learner", &orgID, "running", "ephemeral", 2000, 1024)

	terminal := &terminalModels.Terminal{
		UserID:      "u-trainer",
		MachineSize: "S",
	}

	require.NoError(t, execBeforeCreate(hook, terminal),
		"a personally-held plan is a personal budget — a colleague's session must not consume it")
}

// ---------------------------------------------------------------------------
// Unknown size fails closed
// ---------------------------------------------------------------------------

func TestTerminalBudgetHook_BeforeCreate_UnknownSize_Error(t *testing.T) {
	db := freshTestDB(t)
	plan := budgetPlanInMem("Pro", 8000, 4096, nil)
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
	plan := budgetPlanInMem("Race", 4000, 2048, nil) // exactly one L fits
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
				insertExistingTerminal(t, db, user, nil, "running", "ephemeral", 4000, 2048)
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
