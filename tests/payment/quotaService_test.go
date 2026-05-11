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

// TestQuotaService_CheckUserQuota_WithOrgContext_UsesOrgPlanLimit asserts
// that when an org context is passed to CheckUserQuota, the limit comes
// from that org's plan — even if the user's personal plan has a higher
// resolution priority (which would otherwise win in the global fallback).
//
// This is the QuotaService-level contract relied on by scenarioController
// (#311), but it exercises QuotaService directly: it does NOT cover the
// controller wiring. A real HTTP-level test for PreviewScenario quota
// scoping would belong in tests/scenarios.
func TestQuotaService_CheckUserQuota_WithOrgContext_UsesOrgPlanLimit(t *testing.T) {
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

// TestQuotaService_CheckUserQuota_OrgScopedCount_DoesNotLeakAcrossOrgs locks
// in the I2 fix: when a user is a member of two orgs (each with its own cap),
// the concurrent_terminals count used by CheckUserQuota must be scoped to
// the org that owns the resolved plan. A global count would let a user blow
// past one org's cap by occupying slots in a different org.
func TestQuotaService_CheckUserQuota_OrgScopedCount_DoesNotLeakAcrossOrgs(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "quota-multi-org-user"

	// Two orgs, each capped at 1 concurrent terminal.
	planX := createPlan(t, db, "OrgXPlan", 10, 1)
	planY := createPlan(t, db, "OrgYPlan", 10, 1)
	orgX, _ := createOrgWithSubscriptionAndType(t, db, "org-x", userID, planX, organizationModels.OrgTypeTeam)
	orgY, _ := createOrgWithSubscriptionAndType(t, db, "org-y", userID, planY, organizationModels.OrgTypeTeam)

	// One terminal in each org — each org is at its cap, but globally the user has 2.
	db.Exec("INSERT INTO terminals (id, user_id, status, organization_id) VALUES (?, ?, ?, ?)",
		uuid.New().String(), userID, "active", orgX.ID.String())
	db.Exec("INSERT INTO terminals (id, user_id, status, organization_id) VALUES (?, ?, ?, ?)",
		uuid.New().String(), userID, "active", orgY.ID.String())

	eps := services.NewEffectivePlanService(db)
	svc := services.NewQuotaService(db, eps)

	// Org X scope — count must be 1 (only the terminal in X), not 2.
	checkX, errX := svc.CheckUserQuota(userID, &orgX.ID, "concurrent_terminals", 1)
	assert.NoError(t, errX)
	assert.Equal(t, int64(1), checkX.CurrentUsage, "count must be scoped to org X only")
	assert.Equal(t, int64(1), checkX.Limit)
	assert.False(t, checkX.Allowed, "user is already at org X cap")

	// Org Y scope — count must be 1 (only the terminal in Y), not 2.
	checkY, errY := svc.CheckUserQuota(userID, &orgY.ID, "concurrent_terminals", 1)
	assert.NoError(t, errY)
	assert.Equal(t, int64(1), checkY.CurrentUsage, "count must be scoped to org Y only")
	assert.Equal(t, int64(1), checkY.Limit)
	assert.False(t, checkY.Allowed, "user is already at org Y cap")
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
