// tests/payment/effectivePlanForOrg_test.go
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
	"gorm.io/gorm"
)

// createOrgWithSubscriptionAndType creates an org with a specific type and subscription.
func createOrgWithSubscriptionAndType(
	t *testing.T, db *gorm.DB,
	orgName string, userID string,
	plan *models.SubscriptionPlan,
	orgType organizationModels.OrganizationType,
) (*organizationModels.Organization, *models.OrganizationSubscription) {
	t.Helper()

	org := &organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             orgName,
		DisplayName:      orgName + " Display",
		OwnerUserID:      userID,
		IsActive:         true,
		OrganizationType: orgType,
		IsPersonal:       orgType == organizationModels.OrgTypePersonal,
	}
	err := db.Omit("Metadata").Create(org).Error
	assert.NoError(t, err)

	member := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           "owner",
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err = db.Omit("Metadata").Create(member).Error
	assert.NoError(t, err)

	orgSub := &models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
		Quantity:           1,
	}
	err = db.Create(orgSub).Error
	assert.NoError(t, err)

	return org, orgSub
}

// createPersonalOrgWithoutSubscription creates a personal org without any subscription.
func createPersonalOrgWithoutSubscription(t *testing.T, db *gorm.DB, orgName string, userID string) *organizationModels.Organization {
	t.Helper()

	org := &organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             orgName,
		DisplayName:      orgName + " Display",
		OwnerUserID:      userID,
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypePersonal,
		IsPersonal:       true,
	}
	err := db.Omit("Metadata").Create(org).Error
	assert.NoError(t, err)

	member := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           "owner",
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err = db.Omit("Metadata").Create(member).Error
	assert.NoError(t, err)

	return org
}

// createTeamOrgWithoutSubscription creates a team org without any subscription.
func createTeamOrgWithoutSubscription(t *testing.T, db *gorm.DB, orgName string, userID string) *organizationModels.Organization {
	t.Helper()

	org := &organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             orgName,
		DisplayName:      orgName + " Display",
		OwnerUserID:      userID,
		IsActive:         true,
		OrganizationType: organizationModels.OrgTypeTeam,
		IsPersonal:       false,
	}
	err := db.Omit("Metadata").Create(org).Error
	assert.NoError(t, err)

	member := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           "owner",
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	err = db.Omit("Metadata").Create(member).Error
	assert.NoError(t, err)

	return org
}

// --- GetUserEffectivePlanForOrg tests ---

func TestGetUserEffectivePlanForOrg_NilOrg_FallsBackToGlobal(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-nil-org"

	proPlan := createPlan(t, db, "Pro", 10, 5)
	createUserSubscription(t, db, userID, proPlan)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlanForOrg(userID, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, proPlan.ID, result.Plan.ID)
	assert.Equal(t, services.PlanSourcePersonal, result.Source)
}

func TestGetUserEffectivePlanForOrg_PersonalOrg_ReturnsPersonalPlan(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-personal-org"

	personalPlan := createPlan(t, db, "PersonalPlan", 5, 2)
	teamPlan := createPlan(t, db, "TeamPlan", 20, 10)

	createUserSubscription(t, db, userID, personalPlan)
	personalOrg := createPersonalOrgWithoutSubscription(t, db, "my-personal", userID)
	createOrgWithSubscriptionAndType(t, db, "my-team", userID, teamPlan, organizationModels.OrgTypeTeam)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlanForOrg(userID, &personalOrg.ID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, services.PlanSourcePersonal, result.Source)
	assert.Equal(t, personalPlan.ID, result.Plan.ID)
	assert.Equal(t, "PersonalPlan", result.Plan.Name)
	assert.NotNil(t, result.UserSubscription)
	assert.Nil(t, result.OrganizationSubscription)
}

func TestGetUserEffectivePlanForOrg_TeamOrg_ReturnsOrgPlan(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-team-org"

	personalPlan := createPlan(t, db, "PersonalPlan", 5, 2)
	teamPlan := createPlan(t, db, "TeamPlan", 20, 10)

	createUserSubscription(t, db, userID, personalPlan)
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "my-team", userID, teamPlan, organizationModels.OrgTypeTeam)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlanForOrg(userID, &teamOrg.ID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
	assert.Equal(t, teamPlan.ID, result.Plan.ID)
	assert.Equal(t, "TeamPlan", result.Plan.Name)
	assert.Nil(t, result.UserSubscription)
	assert.NotNil(t, result.OrganizationSubscription)
}

func TestGetUserEffectivePlanForOrg_PersonalOrg_NoPersonalSub_Error(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-no-personal-sub"

	// Create a personal org but no personal subscription
	personalOrg := createPersonalOrgWithoutSubscription(t, db, "my-personal", userID)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlanForOrg(userID, &personalOrg.ID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no active personal subscription")
}

func TestGetUserEffectivePlanForOrg_TeamOrg_NoSubscription_Error(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-team-no-sub"

	// Create a team org without any subscription
	teamOrg := createTeamOrgWithoutSubscription(t, db, "my-team", userID)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlanForOrg(userID, &teamOrg.ID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no active subscription for organization")
}

func TestGetUserEffectivePlanForOrg_NonexistentOrg_Error(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-nonexistent-org"

	fakeOrgID := uuid.New()

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlanForOrg(userID, &fakeOrgID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to load organization")
}

// --- CheckEffectiveUsageLimitForOrg tests ---

func TestCheckEffectiveUsageLimitForOrg_TeamOrg_UsesOrgPlanLimits(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-org-limit-for-org"

	teamPlan := createPlan(t, db, "TeamPlan", 20, 3) // max 3 concurrent terminals
	teamOrg, _ := createOrgWithSubscriptionAndType(t, db, "my-team", userID, teamPlan, organizationModels.OrgTypeTeam)

	// Insert 2 active terminals
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimitForOrg(userID, &teamOrg.ID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.True(t, check.Allowed)
	assert.Equal(t, int64(2), check.CurrentUsage)
	assert.Equal(t, int64(3), check.Limit)
	assert.Equal(t, int64(1), check.RemainingUsage)
}

func TestCheckEffectiveUsageLimitForOrg_NilOrg_FallsBackToGlobal(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-nil-org-limit"

	plan := createPlan(t, db, "PersonalPlan", 5, 2)
	createUserSubscription(t, db, userID, plan)

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimitForOrg(userID, nil, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.True(t, check.Allowed)
	assert.Equal(t, int64(2), check.Limit)
}
