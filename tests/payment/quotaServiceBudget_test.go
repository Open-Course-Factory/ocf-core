// tests/payment/quotaServiceBudget_test.go
//
// Tests for the budget-based methods on QuotaService.
//
// The service sums per-terminal CPU/RAM footprints against the plan's
// MaxCPU/MaxMemoryMB caps.
//
// Lifecycle rule under test (D6' — supersedes D6, locked 2026-05-28):
// a terminal counts against the budget iff state IN ('running','stopped')
// AND deleted_at IS NULL AND expires_at > NOW(). The persistence_mode
// distinction is irrelevant for quota — "a stop is a stop". Stopped
// rows reserve their slot until tt-backend confirms the container is
// gone (sync's step 5b then marks the row deleted).
package payment_tests

import (
	"math"
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// budgetPlan builds a SubscriptionPlan tuned for budget-mode tests.
// maxCPU=0 / maxMem=0 mean unlimited per the contract.
//
// maxCPU is in millicores (mCPU): 1000 mCPU = 1 vCPU. Callers passing
// integer vCPU values (legacy fixtures) must multiply by 1000.
//
// budgetPlan builds a plan with the given CPU/RAM caps. The trailing
// []string parameter is retained for call-site compatibility (it used to
// carry AllowedMachineSizes); it is now ignored.
func budgetPlan(t *testing.T, db *gorm.DB, name string, maxCPU, maxMemMB int, _ []string) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            name,
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		MaxCPU:          maxCPU,
		MaxMemoryMB:     maxMemMB,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// insertTerminal inserts a row with the columns budget logic depends on.
// state and persistence_mode are the new lifecycle fields; size_cpu /
// size_memory_mb are the denormalised footprint columns from MR-CORE-3.
//
// The schema default for expires_at ('2099-12-31') already satisfies the
// `expires_at > NOW()` filter in sumActiveResources*, so callers that
// want a "live" terminal do not need to set it explicitly.
// insertTerminalWithExpiry exists for tests that need to exercise the
// past-expiry exclusion path. MR !239 dropped the legacy `status` column —
// it is no longer written here.
func insertTerminal(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID, state, persistence string, cpu, memMB int) {
	t.Helper()
	id := uuid.New().String()
	var orgStr any
	if orgID != nil {
		orgStr = orgID.String()
	} else {
		orgStr = nil
	}
	err := db.Exec(`INSERT INTO terminals
		(id, user_id, organization_id, state, persistence_mode, size_cpu, size_memory_mb)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, userID, orgStr, state, persistence, cpu, memMB).Error
	require.NoError(t, err)
}

// insertTerminalWithExpiry is the same as insertTerminal but lets the
// caller pin expires_at. Used by past-expiry tests to verify that the
// `expires_at > NOW()` predicate excludes zombie rows from the budget
// sum (mirrors OccupiesSlotScope's zombie-exclusion rule).
func insertTerminalWithExpiry(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID, state, persistence string, cpu, memMB int, expiresAt time.Time) {
	t.Helper()
	id := uuid.New().String()
	var orgStr any
	if orgID != nil {
		orgStr = orgID.String()
	} else {
		orgStr = nil
	}
	err := db.Exec(`INSERT INTO terminals
		(id, user_id, organization_id, state, persistence_mode, size_cpu, size_memory_mb, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, userID, orgStr, state, persistence, cpu, memMB, expiresAt).Error
	require.NoError(t, err)
}

func newQuotaSvc(t *testing.T, db *gorm.DB) services.QuotaService {
	t.Helper()
	eps := services.NewEffectivePlanService(db)
	return services.NewQuotaService(db, eps)
}

// --- CheckBudget --------------------------------------------------------

func TestQuotaService_CheckBudget_BindingAxisCPU(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "u-budget-cpu-axis"

	// 4 vCPU / 8 GiB budget; no usage yet; requesting size L (4000 mCPU / 2g).
	plan := budgetPlan(t, db, "BudgetCPU", 4000, 8192, nil)

	check, err := newQuotaSvc(t, db).CheckBudget(userID, nil, plan, 4000, 2048)

	require.NoError(t, err)
	require.NotNil(t, check)
	assert.True(t, check.Allowed, "L (4000mCPU/2g) fits in a 4000mCPU/8g budget with no usage")
	assert.Equal(t, 0, check.RemainingCPU, "after this request would consume the full CPU axis")
	assert.Equal(t, 6144, check.RemainingMemMB, "memory remaining = 8192 - 2048 = 6144 MiB")
	assert.Empty(t, check.Reason)
}

func TestQuotaService_CheckBudget_BindingAxisRAM(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "u-budget-ram-axis"

	// 8 vCPU / 2 GiB budget; requesting size L (4000 mCPU / 2g) — RAM is
	// the binding axis.
	plan := budgetPlan(t, db, "BudgetRAM", 8000, 2048, nil)

	check, err := newQuotaSvc(t, db).CheckBudget(userID, nil, plan, 4000, 2048)

	require.NoError(t, err)
	require.NotNil(t, check)
	assert.True(t, check.Allowed)
	assert.Equal(t, 4000, check.RemainingCPU)
	assert.Equal(t, 0, check.RemainingMemMB, "RAM is the binding axis")
}

func TestQuotaService_CheckBudget_RejectsOverBudget_CPU(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "u-budget-cpu-reject"

	// 4 vCPU / 8 GiB budget; 4000 mCPU already used; request L
	// (4000 mCPU / 2g) → reject.
	plan := budgetPlan(t, db, "BudgetCPUReject", 4000, 8192, nil)
	insertTerminal(t, db, userID, nil, "running", "ephemeral", 4000, 2048)

	check, err := newQuotaSvc(t, db).CheckBudget(userID, nil, plan, 4000, 2048)

	require.NoError(t, err)
	require.NotNil(t, check)
	assert.False(t, check.Allowed)
	assert.Equal(t, "budget_cpu_exceeded", check.Reason)
}

func TestQuotaService_CheckBudget_RejectsOverBudget_Memory(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "u-budget-mem-reject"

	// 8 vCPU / 2 GiB budget; 2 GiB used; request L (4000 mCPU / 2g) → reject.
	plan := budgetPlan(t, db, "BudgetMemReject", 8000, 2048, nil)
	insertTerminal(t, db, userID, nil, "running", "ephemeral", 0, 2048)

	check, err := newQuotaSvc(t, db).CheckBudget(userID, nil, plan, 4000, 2048)

	require.NoError(t, err)
	require.NotNil(t, check)
	assert.False(t, check.Allowed)
	assert.Equal(t, "budget_memory_exceeded", check.Reason)
}

func TestQuotaService_CheckBudget_MixedActiveSessions(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "u-budget-mixed"

	// Budget: 8 vCPU / 4 GiB = 8000 mCPU / 4096 MiB.
	// Used: 1 M (2000 mCPU / 1024MiB) + 1 S (1000 mCPU / 512MiB)
	//       = 3000 mCPU / 1536MiB.
	// Request: L (4000 mCPU / 2048MiB). Remaining post-check:
	//   5000-4000=1000 mCPU, 2560-2048=512MiB → fits.
	plan := budgetPlan(t, db, "BudgetMixed", 8000, 4096, nil)
	insertTerminal(t, db, userID, nil, "running", "ephemeral", 2000, 1024)
	insertTerminal(t, db, userID, nil, "running", "ephemeral", 1000, 512)

	check, err := newQuotaSvc(t, db).CheckBudget(userID, nil, plan, 4000, 2048)

	require.NoError(t, err)
	require.NotNil(t, check)
	assert.True(t, check.Allowed)
	assert.Equal(t, 1000, check.RemainingCPU, "8000 - (2000+1000) - 4000 = 1000")
	assert.Equal(t, 512, check.RemainingMemMB, "4096 - (1024+512) - 2048 = 512")
}

func TestQuotaService_CheckBudget_ZeroBudget_Unlimited(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "u-budget-unlimited"

	// MaxCPU = 0 and MaxMemoryMB = 0 → unlimited. Even XL (4000 mCPU / 4g)
	// is fine.
	plan := budgetPlan(t, db, "BudgetUnlimited", 0, 0, nil)

	check, err := newQuotaSvc(t, db).CheckBudget(userID, nil, plan, 4000, 4096)

	require.NoError(t, err)
	require.NotNil(t, check)
	assert.True(t, check.Allowed)
	assert.Equal(t, math.MaxInt32, check.RemainingCPU, "unlimited budget reports a sentinel")
	assert.Equal(t, math.MaxInt32, check.RemainingMemMB)
}

// TestQuotaService_CheckBudget_StoppedCountsRegardlessOfPersistence pins the
// new rule (D6', supersedes D6): every stopped session — persistent OR
// ephemeral — counts against the budget until tt-backend confirms the
// container is gone (at which point sync's step 5b marks the row deleted).
// The persistence_mode distinction is UX-only, not budget logic.
func TestQuotaService_CheckBudget_StoppedCountsRegardlessOfPersistence(t *testing.T) {
	cases := []struct {
		name        string
		persistence string
	}{
		{"persistent", "persistent"},
		{"ephemeral", "ephemeral"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			ensureTerminalsTable(t, db)
			userID := "u-budget-stopped-" + tc.persistence

			// Budget: 4 vCPU / 2 GiB = 4000 mCPU / 2048 MiB. One stopped
			// session of size M (2000 mCPU / 1g) in the persistence mode
			// under test. Request L (4000 mCPU / 2g): 2000 used + 4000
			// requested > 4000 cap → reject.
			plan := budgetPlan(t, db, "BudgetStopped-"+tc.persistence, 4000, 2048, nil)
			insertTerminal(t, db, userID, nil, "stopped", tc.persistence, 2000, 1024)

			check, err := newQuotaSvc(t, db).CheckBudget(userID, nil, plan, 4000, 2048)
			require.NoError(t, err)
			require.NotNil(t, check)
			assert.False(t, check.Allowed,
				"a stop is a stop: stopped %s session must count against budget", tc.persistence)

			// And GetBudgetUsage must reflect the reservation (the dashboard
			// /Utilisation Actuelle bars read through the same path).
			usedCPU, usedMem, err := newQuotaSvc(t, db).GetBudgetUsage(userID, nil)
			require.NoError(t, err)
			assert.Equal(t, 2000, usedCPU,
				"stopped %s session must report CPU usage in mCPU", tc.persistence)
			assert.Equal(t, 1024, usedMem,
				"stopped %s session must report memory usage", tc.persistence)
		})
	}
}

// Past-expiry zombie rows must not count against the budget. Mirrors the
// `expires_at > NOW()` clause added to OccupiesSlotScope in MR !239: a row
// whose proxy session is long gone but whose state column was never reset
// must not keep eating budget. Without this, "you have 0 visible sessions
// but the budget is full" — the exact failure mode the slot count had
// before the SSOT consolidation.
func TestQuotaService_CheckBudget_PastExpirySessionsDoNotCount(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "u-budget-past-expiry"

	plan := budgetPlan(t, db, "BudgetPastExpiry", 4000, 2048, nil)

	// A persistent terminal whose lifecycle says "should count" (state=running)
	// but whose expires_at is in the past. The new expires_at > NOW() filter
	// must exclude it, freeing the whole budget for the new request.
	pastExpiry := time.Now().Add(-1 * time.Hour)
	insertTerminalWithExpiry(t, db, userID, nil, "running", "persistent", 4000, 2048, pastExpiry)

	check, err := newQuotaSvc(t, db).CheckBudget(userID, nil, plan, 4000, 2048)

	require.NoError(t, err)
	require.NotNil(t, check)
	assert.True(t, check.Allowed, "past-expiry session must NOT count against budget")
	// After granting the request the budget is exactly consumed. The key
	// assertion is Allowed=true: without the expires_at > NOW() clause, the
	// zombie row would consume the budget and the request would be denied.
	assert.Equal(t, 0, check.RemainingCPU, "after grant, CPU budget is exactly consumed (4000-0-4000)")
	assert.Equal(t, 0, check.RemainingMemMB, "after grant, memory budget is exactly consumed (2048-0-2048)")
}

func TestQuotaService_CheckBudget_OrgScoped(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	owner := "u-budget-org-owner"
	user2 := "u-budget-org-2"
	user3 := "u-budget-org-3"

	// Team plan with budget mode. Three users in the org; two have a running
	// terminal of size M (2000 mCPU / 1g). The third user checks budget —
	// sums must include all three users (total 4000 mCPU / 2g used).
	teamPlan := budgetPlan(t, db, "BudgetTeam", 8000, 4096, nil)
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "budget-team", owner, teamPlan, organizationModels.OrgTypeTeam)

	// Add user2 + user3 as members (helper only added the owner)
	require.NoError(t, db.Omit("Metadata").Create(&organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: teamOrg.ID,
		UserID:         user2,
		Role:           "member",
		JoinedAt:       time.Now(),
		IsActive:       true,
	}).Error)
	require.NoError(t, db.Omit("Metadata").Create(&organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: teamOrg.ID,
		UserID:         user3,
		Role:           "member",
		JoinedAt:       time.Now(),
		IsActive:       true,
	}).Error)

	// owner + user2 each have a running M-sized terminal in the org.
	insertTerminal(t, db, owner, &teamOrg.ID, "running", "ephemeral", 2000, 1024)
	insertTerminal(t, db, user2, &teamOrg.ID, "running", "ephemeral", 2000, 1024)

	// user3 asks for an L (4000 mCPU / 2g). Used = 4000 mCPU / 2g;
	// remaining = 4000 mCPU / 2g; fits.
	check, err := newQuotaSvc(t, db).CheckBudget(user3, &teamOrg.ID, teamPlan, 4000, 2048)
	require.NoError(t, err)
	require.NotNil(t, check)
	assert.True(t, check.Allowed)
	assert.Equal(t, 0, check.RemainingCPU)
	assert.Equal(t, 0, check.RemainingMemMB)

	// And an XL request from user3 must be rejected — proving the org-wide
	// sum was applied, not just user3's own (zero) usage.
	checkOver, err := newQuotaSvc(t, db).CheckBudget(user3, &teamOrg.ID, teamPlan, 4000, 4096)
	require.NoError(t, err)
	require.NotNil(t, checkOver)
	assert.False(t, checkOver.Allowed, "org-wide sum must include the other two members")
}

// --- ComputeRemainingBySize --------------------------------------------

func TestQuotaService_ComputeRemainingBySize_AllSizes(t *testing.T) {
	db := freshTestDB(t)

	// Budget: 8 vCPU / 4 GiB = 8000 mCPU / 4096 MiB. Used: 2000 mCPU /
	// 1 GiB (1024 MiB). Remaining axis: 6000 mCPU / 3072 MiB.
	// Per size (CPU in mCPU; floor division):
	//   xs (500 / 256MiB)   → min(6000/500,  3072/256)  = min(12, 12) = 12
	//   s  (1000 / 512MiB)  → min(6000/1000, 3072/512)  = min(6, 6)   = 6
	//   m  (2000 / 1024MiB) → min(6000/2000, 3072/1024) = min(3, 3)   = 3
	//   l  (4000 / 2048MiB) → min(6000/4000, 3072/2048) = min(1, 1)   = 1
	//   xl (4000 / 4096MiB) → min(6000/4000, 3072/4096) = min(1, 0)   = 0
	plan := budgetPlan(t, db, "BudgetRemainings", 8000, 4096, nil)
	svc := newQuotaSvc(t, db)

	remaining := svc.ComputeRemainingBySize(plan, 2000, 1024)

	// Build a map for easy lookup
	got := map[string]services.SizeRemaining{}
	for _, r := range remaining {
		got[r.Key] = r
	}

	require.Contains(t, got, "xs")
	require.Contains(t, got, "s")
	require.Contains(t, got, "m")
	require.Contains(t, got, "l")
	require.Contains(t, got, "xl")

	assert.Equal(t, 12, got["xs"].RemainingCount,
		"XS at 500 mCPU under 6000 mCPU remaining = 12 (post-switch: twice as many as the legacy XS=1 unit)")
	assert.Equal(t, 6, got["s"].RemainingCount)
	assert.Equal(t, 3, got["m"].RemainingCount)
	assert.Equal(t, 1, got["l"].RemainingCount)
	assert.Equal(t, 0, got["xl"].RemainingCount)

	// Spot-check that the CPU/MemoryMB per size are correctly populated
	// too — CPU stamped in mCPU.
	assert.Equal(t, 4000, got["l"].CPU)
	assert.Equal(t, 2048, got["l"].MemoryMB)
}

func TestQuotaService_ComputeRemainingBySize_ZeroBudget_Unlimited(t *testing.T) {
	db := freshTestDB(t)

	plan := budgetPlan(t, db, "BudgetUnlimitedRemainings", 0, 0, nil)
	svc := newQuotaSvc(t, db)

	remaining := svc.ComputeRemainingBySize(plan, 0, 0)

	require.NotEmpty(t, remaining)
	for _, r := range remaining {
		assert.Equal(t, math.MaxInt32, r.RemainingCount, "unlimited budget reports sentinel for every size, got size %s = %d", r.Key, r.RemainingCount)
	}
}

// --- RemainingBudgetFits ------------------------------------------------

func TestQuotaService_RemainingBudgetFits_True(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "u-budget-fits-true"

	// 8 vCPU / 4 GiB = 8000 mCPU / 4096 MiB. No usage. Request "m"
	// (2000 mCPU / 1g). Plenty of room.
	plan := budgetPlan(t, db, "BudgetFitsTrue", 8000, 4096, nil)

	fits, err := newQuotaSvc(t, db).RemainingBudgetFits(userID, nil, plan, "m")
	require.NoError(t, err)
	assert.True(t, fits)
}

func TestQuotaService_RemainingBudgetFits_False(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "u-budget-fits-false"

	// 4 vCPU / 2 GiB = 4000 mCPU / 2048 MiB. One running M
	// (2000 mCPU / 1g). Request "L" (4000 mCPU / 2g). Doesn't fit.
	plan := budgetPlan(t, db, "BudgetFitsFalse", 4000, 2048, nil)
	insertTerminal(t, db, userID, nil, "running", "ephemeral", 2000, 1024)

	fits, err := newQuotaSvc(t, db).RemainingBudgetFits(userID, nil, plan, "l")
	require.NoError(t, err)
	assert.False(t, fits)
}
