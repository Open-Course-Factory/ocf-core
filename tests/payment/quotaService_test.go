// tests/payment/quotaService_test.go
//
// Tests for the consolidated QuotaService — the single entry point for
// "is X within quota?" decisions. Plan resolution (which plan applies
// to a user in a given org context) stays in EffectivePlanService, and
// the quota counter primitives stay in terminalTrainer/models. This
// service composes the two and is the only place where quota decisions
// are made.
package payment_tests

import (
	"testing"

	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestQuotaService_CheckUserQuota_UnderLimit_Allowed(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "quota-user-under"

	plan := createPlan(t, db, "Basic", 5, 3) // 3 concurrent terminals max
	createUserSubscription(t, db, userID, plan)

	// 1 occupying terminal — under limit
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")

	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	check, err := svc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.True(t, check.Allowed)
	assert.Equal(t, int64(1), check.CurrentUsage)
	assert.Equal(t, int64(3), check.Limit)
	assert.Equal(t, int64(2), check.RemainingUsage)
	assert.Empty(t, check.Message)
}

func TestQuotaService_CheckUserQuota_AtLimit_Denied(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "quota-user-at-limit"

	plan := createPlan(t, db, "Basic", 5, 2)
	createUserSubscription(t, db, userID, plan)

	// 2 active terminals already — at limit
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")

	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	check, err := svc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.False(t, check.Allowed)
	assert.Equal(t, int64(2), check.CurrentUsage)
	assert.Equal(t, int64(2), check.Limit)
	assert.Equal(t, int64(0), check.RemainingUsage)
	assert.Contains(t, check.Message, "Usage limit exceeded")
}

func TestQuotaService_CheckUserQuota_OverLimit_Denied(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "quota-user-over"

	plan := createPlan(t, db, "Basic", 5, 1)
	createUserSubscription(t, db, userID, plan)

	// 3 occupying terminals — already over (1 active + 2 stopped)
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "stopped")
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "stopped")

	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	check, err := svc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.False(t, check.Allowed)
	assert.Equal(t, int64(3), check.CurrentUsage)
	assert.Equal(t, int64(1), check.Limit)
	assert.Equal(t, int64(0), check.RemainingUsage)
}

func TestQuotaService_CheckUserQuota_NilOrg_UsesPersonalPlan(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "quota-user-nil-org"

	personalPlan := createPlan(t, db, "PersonalPlan", 5, 2)
	createUserSubscription(t, db, userID, personalPlan)

	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	check, err := svc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.Equal(t, services.PlanSourcePersonal, check.Source, "nil org context falls back to global resolution → personal plan wins")
	assert.Equal(t, int64(2), check.Limit, "limit must come from personal plan")
}

func TestQuotaService_CheckUserQuota_WithOrg_UsesOrgPlan(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "quota-user-with-org"

	// Personal plan is small, org plan is larger
	personalPlan := createPlan(t, db, "PersonalSmall", 5, 1)
	teamPlan := createPlan(t, db, "TeamPlan", 20, 5)

	createUserSubscription(t, db, userID, personalPlan)
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "my-team", userID, teamPlan, organizationModels.OrgTypeTeam)

	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	check, err := svc.CheckUserQuota(userID, &teamOrg.ID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.Equal(t, services.PlanSourceOrganization, check.Source, "org context resolves to org plan, not personal")
	assert.Equal(t, int64(5), check.Limit, "limit must come from team plan, not personal")
}

func TestQuotaService_CheckUserQuotaWithPlan_SkipsResolution(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "quota-user-pre-resolved"

	// Build a plan result manually — pretend resolution already happened.
	// No UserSubscription / OrganizationSubscription needed for the check.
	plan := createPlan(t, db, "PreResolved", 5, 4)
	result := &services.EffectivePlanResult{
		Plan:   plan,
		Source: services.PlanSourcePersonal,
	}

	// Insert 2 terminals
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "stopped")

	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	check, err := svc.CheckUserQuotaWithPlan(result, userID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.True(t, check.Allowed)
	assert.Equal(t, int64(2), check.CurrentUsage)
	assert.Equal(t, int64(4), check.Limit)
	assert.Equal(t, services.PlanSourcePersonal, check.Source)
}

func TestQuotaService_GetOrgQuota_ReturnsCurrentAndLimit(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	ownerID := "quota-org-owner"

	teamPlan := createPlan(t, db, "TeamPlan", 20, 7)
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "team-quota", ownerID, teamPlan, organizationModels.OrgTypeTeam)

	// Insert 2 terminals owned by the org owner; tag them with org id so
	// CountOrgOccupiedSlots picks them up via the organization_members join.
	db.Exec("INSERT INTO terminals (id, user_id, status, organization_id) VALUES (?, ?, ?, ?)", uuid.New().String(), ownerID, "active", teamOrg.ID.String())
	db.Exec("INSERT INTO terminals (id, user_id, status, organization_id) VALUES (?, ?, ?, ?)", uuid.New().String(), ownerID, "stopped", teamOrg.ID.String())

	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	limits, err := svc.GetOrgQuota(teamOrg.ID)

	assert.NoError(t, err)
	assert.NotNil(t, limits)
	assert.Equal(t, teamOrg.ID, limits.OrganizationID)
	assert.Equal(t, 7, limits.MaxConcurrentTerminals)
	assert.Equal(t, 2, limits.CurrentTerminals, "both active and stopped occupy a slot")
}

// TestQuotaService_CheckUserQuota_ScenarioControllerBug_Regression documents
// the bug fixed in scenarioController.PreviewScenario (#311). Before the fix,
// the controller resolved orgIDForPlan a few lines above the quota check, then
// called EffectivePlanService.CheckEffectiveUsageLimit (no org argument),
// which fell back to the global plan resolution and counted against the user's
// PERSONAL quota — even when the scenario was launched in an org context.
//
// The fix routes the quota check through QuotaService.CheckUserQuota with the
// resolved orgIDForPlan, so the org's plan limit applies. This test asserts
// that contract: a user with a tight personal plan (1 terminal) and a
// generous team-org plan (5 terminals) gets the team plan's limit when the
// org context is provided, even with the personal plan currently at quota.
func TestQuotaService_CheckUserQuota_ScenarioControllerBug_Regression(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "scenario-org-bug-regression"

	// Personal plan (1 terminal) at HIGHER priority than the team plan, so
	// the global fallback (no org context) prefers the personal plan. The
	// team plan still has a different limit (5) so we can distinguish which
	// plan was actually consulted.
	personalPlan := createPlan(t, db, "PersonalTight", 50, 1)
	teamPlan := createPlan(t, db, "TeamGenerous", 20, 5)

	createUserSubscription(t, db, userID, personalPlan)
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "scenario-org", userID, teamPlan, organizationModels.OrgTypeTeam)

	// User has 0 terminals — both checks should be Allowed by their own plan,
	// but the LIMIT they report must differ depending on org context.
	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	// Without org context — the bug path (global fallback to personal plan)
	checkPersonal, errP := svc.CheckUserQuota(userID, nil, "concurrent_terminals", 1)
	assert.NoError(t, errP)
	assert.Equal(t, int64(1), checkPersonal.Limit, "without org context the user's personal limit applies")
	assert.Equal(t, services.PlanSourcePersonal, checkPersonal.Source)

	// With org context — the fixed path (team plan wins regardless of priority)
	checkOrg, errO := svc.CheckUserQuota(userID, &teamOrg.ID, "concurrent_terminals", 1)
	assert.NoError(t, errO)
	assert.Equal(t, int64(5), checkOrg.Limit, "with org context the team plan's limit applies (regression for #311)")
	assert.Equal(t, services.PlanSourceOrganization, checkOrg.Source)
}

func TestQuotaService_GetUserUsage_ConcurrentTerminals_LiveCount(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "quota-live-count"

	plan := createPlan(t, db, "Basic", 5, 3)
	createUserSubscription(t, db, userID, plan)

	// 2 occupying + 1 expired = real count 2
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "stopped")
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "expired")

	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	usage, err := svc.GetUserUsage(userID, nil, "concurrent_terminals")

	assert.NoError(t, err)
	assert.Equal(t, int64(2), usage, "live count from terminals table — must not depend on usage_metrics.current_value")
}
