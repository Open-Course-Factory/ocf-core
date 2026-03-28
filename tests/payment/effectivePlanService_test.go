// tests/payment/effectivePlanService_test.go
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

// createPlan is a helper that creates a SubscriptionPlan with the given name, priority, and terminal limit.
func createPlan(t *testing.T, db *gorm.DB, name string, priority int, maxTerminals int) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   name,
		Priority:               priority,
		PriceAmount:            0,
		Currency:               "eur",
		BillingInterval:        "month",
		IsActive:               true,
		MaxConcurrentTerminals: maxTerminals,
		MaxCourses:             5,
	}
	err := db.Create(plan).Error
	assert.NoError(t, err)
	return plan
}

// createUserSubscription is a helper that creates an active personal UserSubscription.
func createUserSubscription(t *testing.T, db *gorm.DB, userID string, plan *models.SubscriptionPlan) *models.UserSubscription {
	t.Helper()
	sub := &models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		SubscriptionType:   "personal",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}
	err := db.Create(sub).Error
	assert.NoError(t, err)
	return sub
}

// createOrgWithSubscription is a helper that creates an Organization, adds the user as a member,
// and creates an active OrganizationSubscription for the given plan.
func createOrgWithSubscription(t *testing.T, db *gorm.DB, orgName string, userID string, plan *models.SubscriptionPlan) (
	*organizationModels.Organization,
	*models.OrganizationSubscription,
) {
	t.Helper()

	org := &organizationModels.Organization{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        orgName,
		DisplayName: orgName + " Display",
		OwnerUserID: userID,
		IsActive:    true,
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

// ensureTerminalsTable creates the terminals table if it doesn't exist (not in main_test.go migration).
func ensureTerminalsTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	err := db.Exec(`CREATE TABLE IF NOT EXISTS terminals (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		status TEXT,
		deleted_at DATETIME
	)`).Error
	assert.NoError(t, err)
	// Clean existing rows
	db.Exec("DELETE FROM terminals")
}

// --- GetUserEffectivePlan tests ---

func TestGetUserEffectivePlan_PersonalOnly(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-personal-only"

	trialPlan := createPlan(t, db, "Trial", 1, 1)
	createUserSubscription(t, db, userID, trialPlan)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, services.PlanSourcePersonal, result.Source)
	assert.Equal(t, trialPlan.ID, result.Plan.ID)
	assert.Equal(t, "Trial", result.Plan.Name)
	assert.NotNil(t, result.UserSubscription)
	assert.Nil(t, result.OrganizationSubscription)
}

func TestGetUserEffectivePlan_OrgOnly(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-org-only"

	proPlan := createPlan(t, db, "Pro", 10, 5)
	createOrgWithSubscription(t, db, "org-pro", userID, proPlan)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
	assert.Equal(t, proPlan.ID, result.Plan.ID)
	assert.Equal(t, "Pro", result.Plan.Name)
	assert.Nil(t, result.UserSubscription)
	assert.NotNil(t, result.OrganizationSubscription)
}

func TestGetUserEffectivePlan_BothPersonalAndOrg_OrgWins(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-both-org-wins"

	trialPlan := createPlan(t, db, "Trial", 1, 1)
	proPlan := createPlan(t, db, "Pro", 10, 5)

	createUserSubscription(t, db, userID, trialPlan)
	createOrgWithSubscription(t, db, "org-pro", userID, proPlan)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
	assert.Equal(t, proPlan.ID, result.Plan.ID)
	assert.Equal(t, "Pro", result.Plan.Name)
	assert.Nil(t, result.UserSubscription)
	assert.NotNil(t, result.OrganizationSubscription)
}

func TestGetUserEffectivePlan_BothPersonalAndOrg_PersonalWins(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-both-personal-wins"

	premiumPlan := createPlan(t, db, "Premium", 20, 10)
	proPlan := createPlan(t, db, "Pro", 10, 5)

	createUserSubscription(t, db, userID, premiumPlan)
	createOrgWithSubscription(t, db, "org-pro", userID, proPlan)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, services.PlanSourcePersonal, result.Source)
	assert.Equal(t, premiumPlan.ID, result.Plan.ID)
	assert.Equal(t, "Premium", result.Plan.Name)
	assert.NotNil(t, result.UserSubscription)
	assert.Nil(t, result.OrganizationSubscription)
}

func TestGetUserEffectivePlan_NoPlan(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-no-plan"

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no active subscription found")
}

func TestGetUserEffectivePlan_MultipleOrgs(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-multi-org"

	basicPlan := createPlan(t, db, "Basic", 5, 2)
	proPlan := createPlan(t, db, "Pro", 10, 5)

	createOrgWithSubscription(t, db, "org-basic", userID, basicPlan)
	createOrgWithSubscription(t, db, "org-pro", userID, proPlan)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, services.PlanSourceOrganization, result.Source)
	assert.Equal(t, proPlan.ID, result.Plan.ID)
	assert.Equal(t, "Pro", result.Plan.Name)
	assert.Equal(t, 10, result.Plan.Priority)
}

func TestGetUserEffectivePlan_EqualPriority_PersonalWins(t *testing.T) {
	db := freshTestDB(t)
	// Create terminals table for this test
	db.Exec("CREATE TABLE IF NOT EXISTS terminals (id TEXT PRIMARY KEY, user_id TEXT, status TEXT, deleted_at DATETIME)")

	userID := "user_equal_priority"

	// Create two plans with the same priority
	personalPlan := createPlan(t, db, "Personal Plan", 10, 2) // priority 10
	orgPlan := createPlan(t, db, "Org Plan", 10, 5)           // same priority 10

	createUserSubscription(t, db, userID, personalPlan)
	createOrgWithSubscription(t, db, "equal-org", userID, orgPlan)

	svc := services.NewEffectivePlanService(db)
	result, err := svc.GetUserEffectivePlan(userID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, services.PlanSourcePersonal, result.Source)
	assert.Equal(t, "Personal Plan", result.Plan.Name)
}

// --- CheckEffectiveUsageLimit tests ---

func TestCheckEffectiveUsageLimit_ConcurrentTerminals_UnderLimit(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-under-limit"

	plan := createPlan(t, db, "Basic", 5, 2)
	createUserSubscription(t, db, userID, plan)

	// No active terminals inserted — current usage = 0

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.True(t, check.Allowed)
	assert.Equal(t, int64(0), check.CurrentUsage)
	assert.Equal(t, int64(2), check.Limit)
	assert.Equal(t, int64(2), check.RemainingUsage)
	assert.Empty(t, check.Message)
}

func TestCheckEffectiveUsageLimit_ConcurrentTerminals_AtLimit(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-at-limit"

	plan := createPlan(t, db, "Basic", 5, 2)
	createUserSubscription(t, db, userID, plan)

	// Insert 2 active terminals
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.False(t, check.Allowed)
	assert.Equal(t, int64(2), check.CurrentUsage)
	assert.Equal(t, int64(2), check.Limit)
	assert.Equal(t, int64(0), check.RemainingUsage)
	assert.Contains(t, check.Message, "Usage limit exceeded")
}

func TestCheckEffectiveUsageLimit_Unlimited(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-unlimited"

	plan := createPlan(t, db, "Enterprise", 30, -1) // -1 = unlimited
	createUserSubscription(t, db, userID, plan)

	// Insert 10 active terminals
	for i := 0; i < 10; i++ {
		db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")
	}

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.True(t, check.Allowed)
	assert.Equal(t, int64(10), check.CurrentUsage)
	assert.Equal(t, int64(-1), check.Limit)
	assert.Equal(t, int64(-1), check.RemainingUsage)
	assert.Empty(t, check.Message)
}
