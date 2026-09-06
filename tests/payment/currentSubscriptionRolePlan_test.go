// tests/payment/currentSubscriptionRolePlan_test.go
//
// A plan that reaches a user through an organization ROLE mapping comes with
// Source=organization and no organization subscription row: the mapping is
// not a subscription. /user-subscriptions/current dereferenced that nil and
// crashed for every student of an organization the moment its member role was
// mapped to a seat plan (2026-09-06, soli-orsys).
package payment_tests

import (
	"testing"

	paymentRoutes "soli/formations/src/payment/routes"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectivePlan_RoleMappingResolvesWithoutASubscriptionRow(t *testing.T) {
	db := freshTestDB(t)
	orgPlan := planWithBudget(t, db, "Formateur classe", 40, 24000, 12288)
	seatPlan := planWithBudget(t, db, "Siege eleve XL", 10, 4000, 4096)
	org := orgSubscriptionOn(t, db, "teacher-1", orgPlan)
	addMemberWithRole(t, db, org.ID, "learner-1", "member")
	rolePlanOn(t, db, org.ID, "member", seatPlan.ID)

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan("learner-1", &org.ID)

	require.NoError(t, err)
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
	assert.Equal(t, seatPlan.ID, result.Plan.ID)
	assert.Nil(t, result.OrganizationSubscription, "a role mapping carries no subscription row: callers must not dereference it")
}

func TestOrganizationPlanToUserDTO_PresentsARoleMappedPlanWithoutASubscription(t *testing.T) {
	db := freshTestDB(t)
	seatPlan := planWithBudget(t, db, "Siege eleve XL", 10, 4000, 4096)

	out := paymentRoutes.OrganizationPlanToUserDTO("learner-1", nil, seatPlan)

	assert.Equal(t, "learner-1", out.UserID)
	assert.Equal(t, seatPlan.ID, out.SubscriptionPlanID)
	assert.Equal(t, "Siege eleve XL", out.SubscriptionPlan.Name)
	assert.Equal(t, "active", out.Status)
	assert.Equal(t, "organization", out.SubscriptionType)
	assert.True(t, out.IsPrimary)
	assert.Equal(t, uuid.Nil, out.ID, "no subscription row, no id to report")
}

func TestOrganizationPlanToUserDTO_KeepsTheSubscriptionFieldsWhenThereIsOne(t *testing.T) {
	db := freshTestDB(t)
	orgPlan := planWithBudget(t, db, "Formateur classe", 40, 24000, 12288)
	org := orgSubscriptionOn(t, db, "teacher-1", orgPlan)
	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan("teacher-1", &org.ID)
	require.NoError(t, err)
	require.NotNil(t, result.OrganizationSubscription)

	out := paymentRoutes.OrganizationPlanToUserDTO("teacher-1", result.OrganizationSubscription, result.Plan)

	assert.Equal(t, result.OrganizationSubscription.ID, out.ID)
	assert.Equal(t, result.OrganizationSubscription.Status, out.Status)
	assert.Equal(t, "Formateur classe", out.SubscriptionPlan.Name)
}
