// tests/payment/userEffectiveFeatures_test.go
//
// #451: the features endpoints could not see a trainer's plan, and could emit
// an empty one.
//
// Both resolved organization subscriptions directly instead of going through
// EffectivePlanService, so neither saw the inheritance the platform now runs on
// — a team org holds no plan of its own and uses the acting member's.
//
// Found by walking the trainer journey against a running server: Marc holds an
// active Formateur, and GET /users/me/features returned
// {"name": "", "id": "00000000-...", "features": []}. ocf-front feeds that list
// into its gray-out logic, so every gated feature read as unavailable.
//
// Two distinct faults, both covered here:
//  1. a personal plan is invisible, because only org subscriptions are aggregated
//  2. an org subscription whose plan was soft-deleted yields a zero-value plan,
//     which was then converted and served as if real — the same defect !333
//     fixed one function along
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

// orgSubscriptionOn wires an active org subscription and its member row.
func orgSubscriptionOn(t *testing.T, db *gorm.DB, userID string,
	plan *models.SubscriptionPlan) *organizationModels.Organization {
	t.Helper()

	org := teamOrgWithoutSubscription(t, db, "corp-"+uuid.New().String()[:8], userID)
	require.NoError(t, db.Create(&models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
	}).Error)
	return org
}

// TestUserEffectiveFeatures_SeesThePersonalPlan is Marc's case: his plan is
// personal, so an aggregation over organizations alone cannot find it.
func TestUserEffectiveFeatures_SeesThePersonalPlan(t *testing.T) {
	db := freshTestDB(t)
	userID := "trainer-marc"

	formateur := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Formateur",
		PriceAmount:            1990,
		Currency:               "eur",
		Priority:               20,
		IsActive:               true,
		GroupManagementEnabled: true,
	}
	require.NoError(t, db.Create(formateur).Error)
	personalSubscription(t, db, userID, formateur)

	got, err := services.NewOrganizationSubscriptionService(db).GetUserEffectiveFeatures(userID)

	require.NoError(t, err, "a trainer with a paid personal plan has features; this used to "+
		"error with 'user has no organization subscriptions'")
	require.NotNil(t, got.HighestPlan)
	assert.Equal(t, "Formateur", got.HighestPlan.Name)
	assert.True(t, got.HasPersonalSubscription)
	assert.NotEmpty(t, got.AllFeatures, "an empty feature list greys out every gated feature")
}

// TestUserEffectiveFeatures_IgnoresASoftDeletedPlan — the zero-value plan. GORM's
// Preload honours the soft delete, leaving the association empty, and that must
// not become the answer.
func TestUserEffectiveFeatures_IgnoresASoftDeletedPlan(t *testing.T) {
	db := freshTestDB(t)
	userID := "member-of-stale-org"

	stale := seedPlan(t, db, "Trainer Plan", 1200)
	orgSubscriptionOn(t, db, userID, stale)
	require.NoError(t, db.Delete(stale).Error) // soft delete, exactly as in the real data

	got, err := services.NewOrganizationSubscriptionService(db).GetUserEffectiveFeatures(userID)

	if err == nil {
		assert.Nil(t, got.HighestPlan,
			"a plan that failed to load is not a plan — serving it as one is how "+
				`{"name": "", "id": "00000000-..."} reached the frontend`)
	}
}

// TestUserEffectiveFeatures_PersonalPlanCanOutrankTheOrg — a trainer on Formateur
// who also belongs to someone else's Trial org must keep his own entitlements.
func TestUserEffectiveFeatures_PersonalPlanCanOutrankTheOrg(t *testing.T) {
	db := freshTestDB(t)
	userID := "trainer-in-a-trial-org"

	formateur := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Formateur",
		PriceAmount:            1990,
		Currency:               "eur",
		Priority:               20,
		IsActive:               true,
		GroupManagementEnabled: true,
	}
	require.NoError(t, db.Create(formateur).Error)
	personalSubscription(t, db, userID, formateur)

	trial := seedPlan(t, db, "Trial", 0)
	orgSubscriptionOn(t, db, userID, trial)

	got, err := services.NewOrganizationSubscriptionService(db).GetUserEffectiveFeatures(userID)

	require.NoError(t, err)
	require.NotNil(t, got.HighestPlan)
	assert.Equal(t, "Formateur", got.HighestPlan.Name,
		"priority decides, and the plan he pays for outranks a free org plan")
}

// TestOrganizationFeatures_FollowsInheritance — the org endpoint must answer the
// same question as the rest of the platform. It used to 404 for a team org with
// no subscription, while session-options happily returned every machine size for
// the same user and org.
func TestOrganizationFeatures_FollowsInheritance(t *testing.T) {
	db := freshTestDB(t)
	userID := "trainer-marc"

	formateur := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Formateur",
		PriceAmount:            1990,
		Currency:               "eur",
		Priority:               20,
		IsActive:               true,
		GroupManagementEnabled: true,
	}
	require.NoError(t, db.Create(formateur).Error)
	personalSubscription(t, db, userID, formateur)
	org := teamOrgWithoutSubscription(t, db, "marc-corp", userID)

	plan, err := services.NewOrganizationSubscriptionService(db).
		GetOrganizationFeatures(org.ID, userID)

	require.NoError(t, err, "the org grants the member's plan; 404 contradicted session-options")
	require.NotNil(t, plan)
	assert.Equal(t, "Formateur", plan.Name)
}
