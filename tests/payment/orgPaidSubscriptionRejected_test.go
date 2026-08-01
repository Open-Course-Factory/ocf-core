// tests/payment/orgPaidSubscriptionRejected_test.go
//
// #450: a paid organization subscription was recorded as `incomplete` and left
// to be "activated by Stripe webhook" — but nothing ever creates a Stripe
// checkout carrying `organization_id`, so that webhook cannot arrive.
//
// Marc has two such rows, 13 seconds apart: he clicked, nothing happened, he
// clicked again, then bought Formateur personally. The UI reported success both
// times.
//
// Decision (2026-07-31): team orgs are never self-service purchasable.
// Structures are "contact us" and get their plan admin-assigned; trainers buy
// personally and their orgs inherit. So the correct response is to refuse the
// request outright rather than to persist a state that can never resolve.
package payment_tests

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedOrgForSubscription(t *testing.T, db *gorm.DB, userID string) *organizationModels.Organization {
	t.Helper()
	org := &organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             "some-corp",
		DisplayName:      "Some Corp",
		OwnerUserID:      userID,
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)
	return org
}

// seedPlan creates a plan suitable for assigning to an ORGANIZATION.
//
// GroupManagementEnabled is set because that is now the rule for org-assignable
// plans (#458): an organization's plan overrides its members' own, so an
// individual plan landing there silently downgrades everyone. Tests that need a
// plan which must be REFUSED should build it inline rather than widening this
// helper.
func seedPlan(t *testing.T, db *gorm.DB, name string, amount int64) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   name,
		PriceAmount:            amount,
		Currency:               "eur",
		IsActive:               true,
		GroupManagementEnabled: true,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

func countOrgSubscriptions(t *testing.T, db *gorm.DB, orgID uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.OrganizationSubscription{}).
		Where("organization_id = ?", orgID).Count(&n).Error)
	return n
}

// TestPaidOrgSubscription_IsRejected is the fix: no unreachable `incomplete` row.
func TestPaidOrgSubscription_IsRejected(t *testing.T) {
	db := freshTestDB(t)
	userID := "trainer"
	org := seedOrgForSubscription(t, db, userID)
	formateur := seedPlan(t, db, "Formateur", 1990)

	sub, err := services.NewOrganizationSubscriptionService(db).
		CreateOrganizationSubscription(org.ID, formateur.ID, userID, false)

	require.Error(t, err, "a paid org plan cannot be self-service purchased — there is no checkout for it")
	assert.Nil(t, sub)
	assert.Zero(t, countOrgSubscriptions(t, db, org.ID),
		"refusing must leave no row behind; an `incomplete` row can never activate and "+
			"shadows nothing but still misreports the org's plan")
}

// TestPaidOrgSubscription_AdminAssignedStillWorks — the "contact us" path for
// structures must be untouched. This is how a school gets its bespoke plan.
func TestPaidOrgSubscription_AdminAssignedStillWorks(t *testing.T) {
	db := freshTestDB(t)
	userID := "school-owner"
	org := seedOrgForSubscription(t, db, userID)
	bespoke := seedPlan(t, db, "École / OF", 49900)

	sub, err := services.NewOrganizationSubscriptionService(db).
		CreateOrganizationSubscription(org.ID, bespoke.ID, userID, true)

	require.NoError(t, err)
	require.NotNil(t, sub)
	assert.Equal(t, "active", sub.Status,
		"admin assignment bypasses Stripe entirely — that is the whole point of 'contact us'")
}

// TestFreeOrgSubscription_StillWorks — a zero-price plan needs no checkout and
// must keep activating immediately.
func TestFreeOrgSubscription_StillWorks(t *testing.T) {
	db := freshTestDB(t)
	userID := "owner"
	org := seedOrgForSubscription(t, db, userID)
	free := seedPlan(t, db, "École / OF (sur devis)", 0)

	sub, err := services.NewOrganizationSubscriptionService(db).
		CreateOrganizationSubscription(org.ID, free.ID, userID, false)

	require.NoError(t, err)
	require.NotNil(t, sub)
	assert.Equal(t, "active", sub.Status)
}

// TestPaidOrgSubscription_LeavesThePlanPointerAlone — #449: the org must not
// come away claiming a plan it was just refused.
func TestPaidOrgSubscription_LeavesThePlanPointerAlone(t *testing.T) {
	db := freshTestDB(t)
	userID := "trainer"
	org := seedOrgForSubscription(t, db, userID)
	formateur := seedPlan(t, db, "Formateur", 1990)

	_, _ = services.NewOrganizationSubscriptionService(db).
		CreateOrganizationSubscription(org.ID, formateur.ID, userID, false)

	var reloaded organizationModels.Organization
	require.NoError(t, db.First(&reloaded, "id = ?", org.ID).Error)
	assert.Nil(t, reloaded.SubscriptionPlanID,
		"marc-corp claimed Formateur while running Trial because the pointer was written "+
			"before the subscription existed")
}
