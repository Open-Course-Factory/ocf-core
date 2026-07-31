// tests/payment/teamOrgNoAutoTrial_test.go
//
// #448: a team org auto-received a free Trial, which then outranked its owner's
// paid personal plan.
//
// Marc bought Formateur (19,90 EUR, active). Inside marc-corp — the org he
// created to run classrooms — his effective plan resolved to Trial, so the
// purchase bought him nothing where he needed it.
//
// resolveForOrg returns the org's subscription whenever one exists and falls
// back to the personal plan only when there is none. An auto-assigned free
// Trial therefore permanently shadows a paid personal plan, on every team org.
//
// The agreed model: a team org is a container for groups, not a billing entity.
// It holds no plan by default and inherits the acting member's entitlement; a
// structure (school/OF) gets one admin-assigned, which legitimately overrides.
//
// These tests pin the inheritance, which is the rule that keeps being expressed
// in two places and drifting.
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// teamOrgWithoutSubscription builds the state the fix produces: a team org whose
// owner is a member, carrying no subscription of its own.
func teamOrgWithoutSubscription(t *testing.T, db *gorm.DB, name, userID string) *organizationModels.Organization {
	t.Helper()

	org := &organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             name,
		DisplayName:      name,
		OwnerUserID:      userID,
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		IsPersonal:       false,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	member := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           "owner",
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	require.NoError(t, db.Omit("Metadata").Create(member).Error)

	return org
}

// personalSubscription gives the user a paid personal plan, the way the working
// self-service checkout does.
func personalSubscription(t *testing.T, db *gorm.DB, userID string, plan *models.SubscriptionPlan) {
	t.Helper()

	sub := &models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		SubscriptionType:   "personal",
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(0, 1, 0),
	}
	require.NoError(t, db.Create(sub).Error)
}

// TestTeamOrgInheritsOwnersPaidPlan is Marc's exact case: the trainer's paid
// plan must be what applies inside the org he created to use it.
func TestTeamOrgInheritsOwnersPaidPlan(t *testing.T) {
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

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan(userID, &org.ID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Plan)
	assert.Equal(t, "Formateur", result.Plan.Name,
		"a team org holds no plan of its own — it must inherit the acting member's entitlement")
	assert.True(t, result.Plan.GroupManagementEnabled,
		"classroom features are exactly what the trainer paid for")
}

// TestTeamOrgAssignedPlanStillOverrides guards the other direction: a structure
// with a bespoke admin-assigned plan must keep it. Inheritance is the default,
// not a rule that erases explicit assignment.
func TestTeamOrgAssignedPlanStillOverrides(t *testing.T) {
	db := freshTestDB(t)
	userID := "school-admin"

	solo := &models.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "Solo",
		PriceAmount: 1200,
		Currency:    "eur",
		Priority:    10,
		IsActive:    true,
	}
	require.NoError(t, db.Create(solo).Error)

	ecole := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "École / OF",
		PriceAmount:            0,
		Currency:               "eur",
		Priority:               30,
		IsActive:               true,
		GroupManagementEnabled: true,
	}
	require.NoError(t, db.Create(ecole).Error)

	personalSubscription(t, db, userID, solo)
	org, _ := createOrgWithSubscriptionAndType(t, db, "esitech", userID, ecole, organizationModels.OrgTypeTeam)

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan(userID, &org.ID)

	require.NoError(t, err)
	require.NotNil(t, result.Plan)
	assert.Equal(t, "École / OF", result.Plan.Name,
		"an explicitly assigned org plan is the structure's bespoke deal and must win")
}
