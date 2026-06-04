// tests/terminalTrainer/orgUsageBudgetSSOT_test.go
//
// Cross-implementation agreement (SSOT) tests for the org terminal-usage
// dashboard.
//
// The org dashboard's CPU/RAM totals and per-user slot counts MUST agree
// with the canonical, ENFORCED source of truth:
//
//   - quotaService.GetBudgetUsage(userID, orgID) — the value the budget gate
//     actually enforces on session start. For an org it sums CPU/RAM through
//     terminalModels.OccupiesSlotScope JOINed via organization_members
//     (member-join).
//   - terminalModels.CountOrgOccupiedSlots(db, orgID) — the org slot count,
//     same OccupiesSlotScope + member-join.
//
// OccupiesSlotScope predicate (SSOT):
//   state IN ('running','stopped') AND deleted_at IS NULL AND expires_at > NOW().
//
// Today GetOrgTerminalUsage computes its own totals with inline queries keyed
// on the terminals.organization_id COLUMN (not the member-join) and a
// divergent per-user CPU/RAM predicate (state='running' OR
// persistence_mode='persistent'). That drifts from the enforced budget:
//   - it MISSES stopped-non-persistent terminals (the budget counts them), and
//   - it INCLUDES org-tagged terminals owned by non-members (the budget,
//     being member-join, excludes them).
//
// In prod this produced an org dashboard reporting ~2x the used CPU/RAM that
// the quota gate enforces, and "0 sessions" next to non-zero CPU. These tests
// pin the dashboard to the enforced numbers; they FAIL today and must PASS once
// GetOrgTerminalUsage delegates to GetBudgetUsage / CountOrgOccupiedSlots.
package terminalTrainer_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"
)

// TestGetOrgTerminalUsage_TotalsMatchEnforcedBudget seeds a deliberate mix of
// terminals and asserts the dashboard's used CPU/RAM and slot count agree with
// the enforced budget (GetBudgetUsage / CountOrgOccupiedSlots), and exclude the
// non-member's org-tagged terminal (proving member-join, not the org column).
func TestGetOrgTerminalUsage_TotalsMatchEnforcedBudget(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Budget plan so GetOrgTerminalUsage emits the Quota envelope with Used*.
	plan := &paymentModels.SubscriptionPlan{
		Name:        "BudgetPro",
		MaxCPU:      100000, // generous so nothing clamps to 0
		MaxMemoryMB: 100000,
		IsActive:    true,
		IsCatalog:   true,
	}
	require.NoError(t, db.Create(plan).Error)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)

	orgSub := &paymentModels.OrganizationSubscription{
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		StripeCustomerID:   "cus_test_" + uuid.New().String()[:8],
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}
	require.NoError(t, db.Create(orgSub).Error)

	// Seed terminals owned by the OWNER (an active member) with a known mix.
	// Counting rows (per OccupiesSlotScope: running/stopped, live, not deleted):
	const (
		runningCPU = 2000
		runningMem = 1024
		stoppedCPU = 1000
		stoppedMem = 512
		persistCPU = 4000
		persistMem = 2048
	)
	// running terminal — counts.
	insertExistingTerminal(t, db, "owner1", &org.ID, "running", "ephemeral", runningCPU, runningMem)
	// stopped, not expired, not persistent — counts under OccupiesSlotScope.
	insertExistingTerminal(t, db, "owner1", &org.ID, "stopped", "ephemeral", stoppedCPU, stoppedMem)
	// stopped + persistent — counts.
	insertExistingTerminal(t, db, "owner1", &org.ID, "stopped", "persistent", persistCPU, persistMem)

	// Non-counting rows:
	// expired terminal (expires_at in the past) — must NOT count.
	insertExistingTerminalWithExpiry(t, db, "owner1", &org.ID, "running", "ephemeral", 9999, 9999, time.Now().Add(-time.Hour))
	// soft-deleted terminal — must NOT count.
	deletedID := uuid.New().String()
	require.NoError(t, db.Exec(`INSERT INTO terminals
		(id, created_at, updated_at, deleted_at, user_id, organization_id, session_id, state, persistence_mode, size_cpu, size_memory_mb, expires_at, machine_size, user_terminal_key_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
		deletedID, time.Now(), time.Now(), time.Now(), "owner1", org.ID.String(),
		"sess-"+deletedID, "running", "ephemeral", 8888, 8888, time.Now().Add(time.Hour), uuid.New().String()).Error)

	// THE KEY ROW: a terminal whose organization_id = this org but owned by a
	// user who is NOT a member of the org. Under member-join (the enforced
	// budget) it must NOT count; under the org-column query it wrongly would.
	insertExistingTerminal(t, db, "non-member", &org.ID, "running", "ephemeral", 7000, 7000)

	// Hand-computed expected sums over the counting rows (owner1 only).
	expectedCPU := runningCPU + stoppedCPU + persistCPU // 7000
	expectedMem := runningMem + stoppedMem + persistMem // 3584
	const expectedSlots = int64(3)

	// Enforced source of truth.
	qs := paymentServices.NewQuotaService(db, nil)
	budgetCPU, budgetMem, err := qs.GetBudgetUsage("", &org.ID)
	require.NoError(t, err)
	require.Equal(t, expectedCPU, budgetCPU, "sanity: enforced budget CPU over member-join")
	require.Equal(t, expectedMem, budgetMem, "sanity: enforced budget Mem over member-join")

	slots, err := terminalModels.CountOrgOccupiedSlots(db, org.ID)
	require.NoError(t, err)
	require.Equal(t, expectedSlots, slots, "sanity: enforced org slot count over member-join")

	// Dashboard under test.
	svc := terminalServices.NewTerminalTrainerService(db)
	resp, err := svc.GetOrgTerminalUsage(org.ID)
	require.NoError(t, err)
	require.NotNil(t, resp.Quota, "budget plan must emit a Quota envelope")

	// Agreement: dashboard Used* == enforced budget == hand-computed sum.
	assert.Equal(t, budgetCPU, resp.Quota.UsedCPU,
		"dashboard UsedCPU must equal the enforced budget CPU")
	assert.Equal(t, budgetMem, resp.Quota.UsedMemoryMB,
		"dashboard UsedMemoryMB must equal the enforced budget Mem")
	assert.Equal(t, expectedCPU, resp.Quota.UsedCPU,
		"dashboard UsedCPU must equal the hand-computed sum over counting rows")
	assert.Equal(t, expectedMem, resp.Quota.UsedMemoryMB,
		"dashboard UsedMemoryMB must equal the hand-computed sum over counting rows")

	// Agreement: sum of per-user occupying slots == enforced slot count.
	var sumOccupying int64
	for _, u := range resp.Users {
		sumOccupying += int64(u.OccupyingSlots)
	}
	assert.Equal(t, slots, sumOccupying,
		"sum of per-user OccupyingSlots must equal CountOrgOccupiedSlots")

	// The non-member's org-tagged terminal's CPU (7000) must NOT be included.
	// If it were, UsedCPU would be 14000 (column semantics) instead of 7000.
	assert.NotEqual(t, expectedCPU+7000, resp.Quota.UsedCPU,
		"non-member org-tagged terminal must be excluded (member-join, not org column)")
	for _, u := range resp.Users {
		assert.NotEqual(t, "non-member", u.UserID,
			"non-member terminal must not appear in the org breakdown")
	}
}

// TestGetOrgTerminalUsage_StoppedSessionCountedAndConsistentWithCpu pins the
// prod "non-zero sessions / disagreeing CPU" bug. A single stopped-EPHEMERAL
// terminal for a member occupies a slot (OccupiesSlotScope counts stopped) and
// carries CPU under the enforced budget. But the dashboard's CPU sum uses the
// divergent `state='running' OR persistence_mode='persistent'` predicate, which
// drops a stopped-ephemeral row — so today it reports a non-zero slot count next
// to ZERO used CPU, the same internal inconsistency users saw in prod.
//
// Contract: we assert on OccupyingSlots (the slot-occupying count) — the field
// that mirrors OccupiesSlotScope and the enforced budget. ActiveTerminals is a
// running-only display field and is intentionally allowed to be 0 here; it is
// the slot count that must not disagree with the budgeted CPU.
func TestGetOrgTerminalUsage_StoppedSessionCountedAndConsistentWithCpu(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	plan := &paymentModels.SubscriptionPlan{
		Name:        "BudgetPro",
		MaxCPU:      100000,
		MaxMemoryMB: 100000,
		IsActive:    true,
		IsCatalog:   true,
	}
	require.NoError(t, db.Create(plan).Error)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)

	orgSub := &paymentModels.OrganizationSubscription{
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		StripeCustomerID:   "cus_test_" + uuid.New().String()[:8],
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}
	require.NoError(t, db.Create(orgSub).Error)

	const stoppedCPU = 2000
	const stoppedMem = 1024
	// A single stopped + EPHEMERAL terminal for a member. It occupies a slot
	// (OccupiesSlotScope counts stopped) but is dropped by the dashboard's
	// running-or-persistent CPU predicate — the exact divergence under test.
	insertExistingTerminal(t, db, "student1", &org.ID, "stopped", "ephemeral", stoppedCPU, stoppedMem)

	// Enforced numbers.
	qs := paymentServices.NewQuotaService(db, nil)
	budgetCPU, budgetMem, err := qs.GetBudgetUsage("", &org.ID)
	require.NoError(t, err)
	require.Equal(t, stoppedCPU, budgetCPU)
	require.Equal(t, stoppedMem, budgetMem)
	slots, err := terminalModels.CountOrgOccupiedSlots(db, org.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), slots)

	svc := terminalServices.NewTerminalTrainerService(db)
	resp, err := svc.GetOrgTerminalUsage(org.ID)
	require.NoError(t, err)
	require.NotNil(t, resp.Quota)

	// CPU/RAM agree with the enforced budget.
	assert.Equal(t, budgetCPU, resp.Quota.UsedCPU)
	assert.Equal(t, budgetMem, resp.Quota.UsedMemoryMB)

	// The headline slot-occupying count must be >= 1 (NOT 0) — the count and
	// the CPU cannot disagree.
	assert.GreaterOrEqual(t, resp.OccupyingSlots, 1,
		"a stopped session that occupies a slot must be counted, not reported as 0")

	// And the count must equal the enforced slot count.
	assert.Equal(t, int(slots), resp.OccupyingSlots,
		"headline occupying count must equal CountOrgOccupiedSlots")
}
