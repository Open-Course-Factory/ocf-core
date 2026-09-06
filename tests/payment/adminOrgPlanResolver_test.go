package payment_tests

// The administrator fallback in InjectEffectivePlan used to be a fourth plan
// resolver with its own query of "the organization's entitling subscription".
// It now goes through EffectivePlanService.GetOrganizationPlan, so an admin
// who is not a member sees exactly the plan — and the budget scope — a member
// would.

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentMiddleware "soli/formations/src/payment/middleware"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInjectEffectivePlan_AdminNonMember_ResolvesOrgPlanWithOrgScope(t *testing.T) {
	db := freshTestDB(t)
	adminID := "platform-admin"

	plan := livePlan(t, db, "School Pool", 20)
	org := teamOrgWithoutSubscription(t, db, "pool-school", "someone-else")
	require.NoError(t, db.Create(&models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
	}).Error)

	ctx, _ := newTestContext("GET", "/terminals/session-options", nil, adminID, []string{"administrator"})
	ctx.Set("org_context_id", org.ID.String())

	paymentMiddleware.InjectEffectivePlan(services.NewEffectivePlanService(db))(ctx)

	val, exists := ctx.Get("effective_plan_result")
	require.True(t, exists)
	result, _ := val.(*services.EffectivePlanResult)
	require.NotNil(t, result, "an administrator acting in an organization gets that organization's plan")
	assert.Equal(t, plan.ID, result.Plan.ID)
	assert.Equal(t, plan.MaxCPU, result.Plan.MaxCPU, "the budget travels with the plan")
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
	require.NotNil(t, result.OrganizationSubscription)
}

func TestGetOrganizationPlan_NoSubscription_Errors(t *testing.T) {
	db := freshTestDB(t)
	org := teamOrgWithoutSubscription(t, db, "bare-org", "owner")

	result, err := services.NewEffectivePlanService(db).GetOrganizationPlan(org.ID)

	require.Error(t, err)
	assert.Nil(t, result)
}

func TestGetOrganizationPlan_DanglingPlan_FailsClosed(t *testing.T) {
	db := freshTestDB(t)
	gone := danglingPlan(t, db, "Retired Org Plan")
	org := teamOrgWithoutSubscription(t, db, "broken-org", "owner")
	require.NoError(t, db.Create(&models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		SubscriptionPlanID: gone.ID,
		Status:             "active",
	}).Error)

	result, err := services.NewEffectivePlanService(db).GetOrganizationPlan(org.ID)

	require.Error(t, err)
	assert.Nil(t, result)
}
