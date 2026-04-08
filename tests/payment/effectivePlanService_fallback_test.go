// tests/payment/effectivePlanService_fallback_test.go
package payment_tests

import (
	"testing"

	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
)

// TestEffectivePlanService_TeamOrgNoSubscription_FallsBackToPersonal verifies that when
// a team org has NO subscription but the user has a personal subscription,
// GetUserEffectivePlanForOrg falls back to the personal subscription and marks it as such.
//
// BUG: The current implementation returns an error ("no active subscription for organization")
// instead of falling back. The IsFallback field does not exist yet on EffectivePlanResult.
func TestEffectivePlanService_TeamOrgNoSubscription_FallsBackToPersonal(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-fallback-personal"

	// User has a personal subscription
	personalPlan := createPlan(t, db, "PersonalPro", 10, 5)
	createUserSubscription(t, db, userID, personalPlan)

	// Team org exists but has NO subscription
	teamOrg := createTeamOrgWithoutSubscription(t, db, "team-no-sub", userID)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlanForOrg(userID, &teamOrg.ID)

	// Should succeed (fallback), not error
	assert.NoError(t, err, "should fall back to personal subscription, not error")
	assert.NotNil(t, result, "result should not be nil when personal fallback is available")

	// Should return the personal plan as a fallback
	assert.Equal(t, personalPlan.ID, result.Plan.ID)
	assert.Equal(t, services.PlanSourcePersonal, result.Source)
	assert.NotNil(t, result.UserSubscription)
	assert.Nil(t, result.OrganizationSubscription)

	// IsFallback must be true to indicate this is a fallback, not the org's own plan
	assert.True(t, result.IsFallback, "IsFallback should be true when falling back to personal subscription")
}

// TestEffectivePlanService_TeamOrgWithSubscription_NoFallback verifies that when a team org
// HAS its own subscription, GetUserEffectivePlanForOrg returns the org subscription
// with IsFallback: false.
func TestEffectivePlanService_TeamOrgWithSubscription_NoFallback(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-no-fallback"

	// User has a personal subscription
	personalPlan := createPlan(t, db, "PersonalBasic", 5, 2)
	createUserSubscription(t, db, userID, personalPlan)

	// Team org has its own subscription
	teamPlan := createPlan(t, db, "TeamEnterprise", 20, 10)
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "team-with-sub", userID, teamPlan, organizationModels.OrgTypeTeam)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlanForOrg(userID, &teamOrg.ID)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should return the org's own plan
	assert.Equal(t, teamPlan.ID, result.Plan.ID)
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
	assert.Nil(t, result.UserSubscription)
	assert.NotNil(t, result.OrganizationSubscription)

	// IsFallback must be false — this is the org's actual subscription
	assert.False(t, result.IsFallback, "IsFallback should be false when org has its own subscription")
}

// TestEffectivePlanService_PersonalOrg_NoFallback verifies that when a personal org context
// is used, the personal subscription is returned with IsFallback: false (it's the expected
// source, not a fallback).
func TestEffectivePlanService_PersonalOrg_NoFallback(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-personal-org-no-fallback"

	// User has a personal subscription
	personalPlan := createPlan(t, db, "PersonalStarter", 5, 3)
	createUserSubscription(t, db, userID, personalPlan)

	// Personal org (no org subscription needed — personal orgs use the user's personal sub)
	personalOrg := createPersonalOrgWithoutSubscription(t, db, "my-personal-org", userID)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlanForOrg(userID, &personalOrg.ID)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should return the personal plan directly
	assert.Equal(t, personalPlan.ID, result.Plan.ID)
	assert.Equal(t, services.PlanSourcePersonal, result.Source)
	assert.NotNil(t, result.UserSubscription)
	assert.Nil(t, result.OrganizationSubscription)

	// IsFallback must be false — personal org naturally uses personal subscription
	assert.False(t, result.IsFallback, "IsFallback should be false for personal org context")
}
