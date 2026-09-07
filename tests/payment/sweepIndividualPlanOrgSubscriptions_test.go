// tests/payment/sweepIndividualPlanOrgSubscriptions_test.go
//
// #458 refuses assigning an individual plan to an organization, on both doors,
// but only at creation time. An organization that already held one kept it,
// and because an org's plan overrides its members' own, every member was
// silently capped by it. This sweep removes those leftovers at startup, with
// the same rule the doors apply, so the guard and the sweep cannot disagree.
package payment_tests

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/initialization"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedIndividualPlan is a paid plan without group management: Solo's shape.
func seedIndividualPlan(t *testing.T, db *gorm.DB, name string) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   name,
		PriceAmount:            990,
		Currency:               "eur",
		IsActive:               true,
		GroupManagementEnabled: false,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

func seedRolePlan(t *testing.T, db *gorm.DB, orgID uuid.UUID, role string, plan *models.SubscriptionPlan) {
	t.Helper()
	require.NoError(t, db.Create(&models.OrganizationRolePlan{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     orgID,
		Role:               role,
		SubscriptionPlanID: plan.ID,
	}).Error)
}

func countRolePlans(t *testing.T, db *gorm.DB, orgID uuid.UUID, role string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.OrganizationRolePlan{}).
		Where("organization_id = ? AND role = ?", orgID, role).Count(&n).Error)
	return n
}

// TestSweepIndividualPlan_CancelsOrgSubscriptionOnIndividualPlan is the
// trainer whose team org was given Solo: the subscription is cancelled and
// the denormalised pointer cleared, so the org inherits its members' plans.
func TestSweepIndividualPlan_CancelsOrgSubscriptionOnIndividualPlan(t *testing.T) {
	db := freshTestDB(t)
	solo := seedIndividualPlan(t, db, "Solo")
	org := seedOrgWithSubscription(t, db, organizationModels.OrgTypeTeam, solo)

	initialization.SweepIndividualPlanOrgSubscriptions(db)

	assert.Equal(t, "cancelled", subscriptionStatus(t, db, org.ID),
		"an organization must not keep a plan the assignment door refuses")
	assert.Nil(t, orgPlanPointer(t, db, org.ID),
		"the organization must stop advertising a plan whose subscription is cancelled")
}

// TestSweepIndividualPlan_LeavesOrgPlansAlone pins the boundary: a plan that
// grants group management is what an organization is meant to hold.
func TestSweepIndividualPlan_LeavesOrgPlansAlone(t *testing.T) {
	db := freshTestDB(t)
	formateur := seedPlan(t, db, "Formateur", 2990)
	org := seedOrgWithSubscription(t, db, organizationModels.OrgTypeTeam, formateur)

	initialization.SweepIndividualPlanOrgSubscriptions(db)

	assert.Equal(t, "active", subscriptionStatus(t, db, org.ID))
	require.NotNil(t, orgPlanPointer(t, db, org.ID))
	assert.Equal(t, formateur.ID, *orgPlanPointer(t, db, org.ID))
}

// TestSweepIndividualPlan_RemovesRoleMappingsTheRoleDoorRefuses: a classroom
// role mapped to an individual plan goes; a student seat mapped to one is
// exactly where an individual plan belongs and stays.
func TestSweepIndividualPlan_RemovesRoleMappingsTheRoleDoorRefuses(t *testing.T) {
	db := freshTestDB(t)
	school := seedPlan(t, db, "École", 9900)
	solo := seedIndividualPlan(t, db, "Solo")
	org := seedOrgWithSubscription(t, db, organizationModels.OrgTypeTeam, school)
	seedRolePlan(t, db, org.ID, "manager", solo)
	seedRolePlan(t, db, org.ID, "member", solo)

	initialization.SweepIndividualPlanOrgSubscriptions(db)

	assert.Equal(t, int64(0), countRolePlans(t, db, org.ID, "manager"),
		"a role that runs classes must not stay mapped to an individual plan")
	assert.Equal(t, int64(1), countRolePlans(t, db, org.ID, "member"),
		"a seat mapping to an individual plan is legitimate and must survive")
	assert.Equal(t, "active", subscriptionStatus(t, db, org.ID))
}

// TestSweepIndividualPlan_IsIdempotent: a second run finds nothing and
// changes nothing.
func TestSweepIndividualPlan_IsIdempotent(t *testing.T) {
	db := freshTestDB(t)
	solo := seedIndividualPlan(t, db, "Solo")
	org := seedOrgWithSubscription(t, db, organizationModels.OrgTypeTeam, solo)

	initialization.SweepIndividualPlanOrgSubscriptions(db)
	var first models.OrganizationSubscription
	require.NoError(t, db.Where("organization_id = ?", org.ID).First(&first).Error)

	initialization.SweepIndividualPlanOrgSubscriptions(db)
	var second models.OrganizationSubscription
	require.NoError(t, db.Where("organization_id = ?", org.ID).First(&second).Error)

	assert.Equal(t, "cancelled", second.Status)
	assert.Equal(t, first.CancelledAt, second.CancelledAt, "an already swept subscription is left as it is")
}
