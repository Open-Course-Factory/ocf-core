// tests/payment/checkEffectiveUsageLimit_orgContext_test.go
//
// Pins the org-context contract of the consolidated
// EffectivePlanService.CheckEffectiveUsageLimit resolver:
//
//   - When called with an explicit orgID, the resolver MUST use that org's
//     plan (or the personal fallback if the org has no subscription),
//     regardless of which plan would win in a global priority comparison.
//   - When called with nil orgID, the resolver keeps its legacy
//     globally-highest-priority semantics for callers that genuinely have
//     no org context (e.g. featureMiddleware, featureAccess utilities).
//
// Before consolidation, the dashboard / launcher "display" path and the
// usage gate "/usage/check" path resolved different plans for the same
// user, producing the launcher-vs-gate mismatch (#334). The org-context
// test below would have caught it.
package payment_tests

import (
	"testing"

	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestCheckEffectiveUsageLimit_OrgContext_UsesOrgPlan_NotGlobalWinner verifies
// that passing the org context makes the gate honor THAT org's plan, even when
// a higher-priority plan (here: personal) would win the global resolution.
//
// Seed:
//   - personal plan: priority 10, MaxConcurrentTerminals = 1 — already at cap
//     (1 personal terminal inserted with NULL organization_id).
//   - team org "A" plan: priority 5, MaxConcurrentTerminals = 5 — empty.
//
// QuotaService scopes its slot count to the same org context the plan came
// from: personal plan → global count by userID; org plan → count restricted
// to org_id. So the personal plan reports 1/1 (denied) but the org plan
// reports 0/5 (allowed) for the same user at the same instant.
//
// Pre-fix behavior (single global resolver via the old GetUserEffectivePlan):
//   - The gate picked personal (priority 10 > 5) → 1/1 → allowed=false.
//
// Expected post-fix behavior (org-aware resolver via merged signature):
//   - With orgID=&teamOrg.ID → picks org A's plan → 0/5 → allowed=true.
func TestCheckEffectiveUsageLimit_OrgContext_UsesOrgPlan_NotGlobalWinner(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-org-context-vs-global"

	// Personal plan dominates by priority but is at cap (1 personal terminal seeded).
	personalPlan := createPlan(t, db, "RestrictivePersonal", 10, 1)
	createUserSubscription(t, db, userID, personalPlan)
	// One active personal terminal (no organization_id) — fills the personal cap.
	db.Exec("INSERT INTO terminals (id, user_id, status, state) VALUES (?, ?, ?, ?)",
		uuid.New().String(), userID, "active", "running")

	// Team org has a generous plan with lower priority, and no terminals in its scope.
	orgPlan := createPlan(t, db, "GenerousTeam", 5, 5)
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "team-A", userID, orgPlan, organizationModels.OrgTypeTeam)

	svc := services.NewEffectivePlanService(db)

	// THE asserted behavior: with an explicit org context, the org's plan wins,
	// and the count is scoped to that org (0 terminals in scope).
	check, err := svc.CheckEffectiveUsageLimit(userID, &teamOrg.ID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.True(t, check.Allowed,
		"with explicit org context, gate must use the org's plan (limit 5, 0/5 in scope) — "+
			"not the globally highest-priority personal plan (limit 1, already at cap)")
	assert.Equal(t, services.PlanSourceOrganization, check.Source)
	assert.Equal(t, int64(5), check.Limit)
	assert.Equal(t, int64(0), check.CurrentUsage)
}

// TestCheckEffectiveUsageLimit_NilOrgContext_KeepsGlobalSemantics verifies the
// regression guard for the nil-orgID branch: callers that genuinely have no
// org context (featureMiddleware, featureAccess utilities) must still see the
// globally highest-priority plan.
//
// Same seed as above. With orgID=nil, the personal plan must win — and its
// 1/1 cap must deny the request.
func TestCheckEffectiveUsageLimit_NilOrgContext_KeepsGlobalSemantics(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-nil-context-keeps-global"

	personalPlan := createPlan(t, db, "RestrictivePersonal", 10, 1)
	createUserSubscription(t, db, userID, personalPlan)
	// Fill the personal cap with one active terminal (no organization_id).
	db.Exec("INSERT INTO terminals (id, user_id, status, state) VALUES (?, ?, ?, ?)",
		uuid.New().String(), userID, "active", "running")

	orgPlan := createPlan(t, db, "GenerousTeam", 5, 5)
	createOrgWithSubscriptionAndType(t, db, "team-A", userID, orgPlan, organizationModels.OrgTypeTeam)

	svc := services.NewEffectivePlanService(db)

	// nil orgID → unchanged global semantics → personal plan wins → denied.
	check, err := svc.CheckEffectiveUsageLimit(userID, nil, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.False(t, check.Allowed,
		"with nil org context, gate must keep legacy global-priority behavior "+
			"(personal plan wins → 1/1 → denied)")
	assert.Equal(t, services.PlanSourcePersonal, check.Source)
	assert.Equal(t, int64(1), check.Limit)
	assert.Equal(t, int64(1), check.CurrentUsage)
}
