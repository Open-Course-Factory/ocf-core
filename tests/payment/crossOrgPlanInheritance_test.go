package payment_tests

// #461: a team organization with no subscription falls back to the acting user's
// own plan. That is the trainer case and it must keep working — he creates an org
// to hold his classes and his Formateur plan applies inside it.
//
// The fallback used to resolve the user's globally highest-priority plan, which
// includes plans inherited through membership of OTHER organizations. So a member
// of a school could create their own team org — they are its owner, so every role
// check passes — and run classes and terminals inside it on the school's plan, in
// a workspace the school cannot see or administer.
//
// The distinction these tests pin: a plan you HOLD follows you, a plan you merely
// BENEFIT FROM elsewhere does not.

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

// seedPlanFor creates a plan at the given priority.
func seedPlanFor(t *testing.T, db *gorm.DB, name string, priority int) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   name,
		Priority:               priority,
		IsActive:               true,
		GroupManagementEnabled: true,
		MaxCPU:                 8000,
		MaxMemoryMB:            8192,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// seedOrgOwning creates a team org owned by ownerID, optionally holding a plan,
// and enrols extraMembers as plain members.
func seedOrgOwning(
	t *testing.T,
	db *gorm.DB,
	name string,
	ownerID string,
	plan *models.SubscriptionPlan,
	extraMembers ...string,
) uuid.UUID {
	t.Helper()

	org := &organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             name,
		DisplayName:      name,
		OwnerUserID:      ownerID,
		OrganizationType: organizationModels.OrgTypeTeam,
		IsActive:         true,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	members := append([]string{ownerID}, extraMembers...)
	for i, uid := range members {
		role := organizationModels.OrgRoleMember
		if i == 0 {
			role = organizationModels.OrgRoleOwner
		}
		require.NoError(t, db.Omit("Metadata").Create(&organizationModels.OrganizationMember{
			BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID: org.ID,
			UserID:         uid,
			Role:           role,
			JoinedAt:       time.Now(),
			IsActive:       true,
		}).Error)
	}

	if plan != nil {
		require.NoError(t, db.Create(&models.OrganizationSubscription{
			BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID:     org.ID,
			SubscriptionPlanID: plan.ID,
			StripeCustomerID:   "cus_" + name,
			Status:             "active",
			CurrentPeriodStart: time.Now().Add(-time.Hour),
			CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
		}).Error)
	}

	return org.ID
}

func seedPersonalSubscription(t *testing.T, db *gorm.DB, userID string, plan *models.SubscriptionPlan, subType string) {
	t.Helper()
	require.NoError(t, db.Create(&models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		SubscriptionType:   subType,
		Status:             "active",
		CurrentPeriodStart: time.Now().Add(-time.Hour),
		CurrentPeriodEnd:   time.Now().AddDate(0, 1, 0),
	}).Error)
}

// The leak. A student belongs to a school on a high-priority plan and holds
// nothing of their own. They create their own team org, which owns no
// subscription — and must NOT inherit the school's plan into it.
func TestResolveForOrg_DoesNotCarryAnotherOrgsPlanIntoAnOrgYouCreate(t *testing.T) {
	db := freshTestDB(t)

	const student = "student-karim"

	schoolPlan := seedPlanFor(t, db, "Ecole / OF", 30)
	seedOrgOwning(t, db, "esitech", "school-admin", schoolPlan, student)

	// The student's own organization: they own it, it holds no plan.
	ownOrgID := seedOrgOwning(t, db, "karim-private", student, nil)

	_, err := services.NewEffectivePlanService(db).GetUserEffectivePlan(student, &ownOrgID)

	require.Error(t, err,
		"a member of a school must not re-host the school's plan in an organization "+
			"they create for themselves — the school cannot see or administer it")
}

// The trainer case, which must keep working: his own plan follows him into the
// organization he created to hold his classes.
func TestResolveForOrg_CarriesYourOwnPlanIntoYourOwnOrg(t *testing.T) {
	db := freshTestDB(t)

	const trainer = "trainer-marc"

	formateur := seedPlanFor(t, db, "Formateur", 20)
	seedPersonalSubscription(t, db, trainer, formateur, "personal")

	orgID := seedOrgOwning(t, db, "marc-formations", trainer, nil)

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan(trainer, &orgID)

	require.NoError(t, err)
	require.NotNil(t, result.Plan)
	assert.Equal(t, formateur.ID, result.Plan.ID, "a personally-bought plan follows its owner")
	assert.True(t, result.IsFallback, "the org owns nothing, so this is the personal fallback")
	assert.Nil(t, result.ScopeOrganizationID,
		"a personally-held plan is a personal budget, even inside an organization")
}

// A learner holding an ASSIGNED seat holds it personally, so it must follow them
// the same way — this is how a trainer's learners get their plan inside his org.
func TestResolveForOrg_CarriesAnAssignedSeatIntoAnOrgWithoutAPlan(t *testing.T) {
	db := freshTestDB(t)

	const learner = "learner-with-seat"

	seatPlan := seedPlanFor(t, db, "Learner Seat", 10)
	seedPersonalSubscription(t, db, learner, seatPlan, "assigned")

	orgID := seedOrgOwning(t, db, "marc-formations", "trainer-marc", nil, learner)

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan(learner, &orgID)

	require.NoError(t, err)
	require.NotNil(t, result.Plan)
	assert.Equal(t, seatPlan.ID, result.Plan.ID,
		"an assigned seat is held by the learner and must follow them")
}

// The school itself is unaffected: acting INSIDE the school, its plan applies and
// the budget is the school's shared pool.
func TestResolveForOrg_SchoolMembersStillInheritTheSchoolsPlanInsideIt(t *testing.T) {
	db := freshTestDB(t)

	const student = "student-karim"

	schoolPlan := seedPlanFor(t, db, "Ecole / OF", 30)
	schoolID := seedOrgOwning(t, db, "esitech", "school-admin", schoolPlan, student)

	result, err := services.NewEffectivePlanService(db).GetUserEffectivePlan(student, &schoolID)

	require.NoError(t, err)
	require.NotNil(t, result.Plan)
	assert.Equal(t, schoolPlan.ID, result.Plan.ID)
	require.NotNil(t, result.ScopeOrganizationID)
	assert.Equal(t, schoolID, *result.ScopeOrganizationID,
		"inside the school the budget is the school's shared pool")
}
