// tests/payment/organizationTrialPlan_test.go
// Tests for auto-assigning the Trial subscription plan to team organizations
// when they are created via organizationService.CreateOrganization().
//
// These tests verify that:
// 1. Creating a team org auto-assigns the Trial plan subscription
// 2. Org creation still succeeds if no Trial plan exists (graceful degradation)
// 3. If the creation input already includes a SubscriptionPlanID, the Trial plan is not overridden
package payment_tests

import (
	"testing"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/services"
	"soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// testDBForOrgService returns the shared DB with:
// 1. A GORM callback to omit Metadata JSONB fields for SQLite compatibility
// 2. A mock Casbin enforcer set on casdoor.Enforcer (required by permission grants)
func testDBForOrgService(t *testing.T) *gorm.DB {
	t.Helper()
	db := freshTestDB(t)

	// Install mock Casbin enforcer so permission grant/revoke calls don't panic
	casdoor.Enforcer = mocks.NewMockEnforcer()

	// Register callback to omit Metadata for Organization/OrganizationMember creates
	// (SQLite doesn't support the JSONB type used by Metadata)
	db.Callback().Create().Before("gorm:create").Register("fix_metadata_for_sqlite", func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil {
			return
		}
		tableName := tx.Statement.Schema.Table
		if tableName == "organizations" || tableName == "organization_members" {
			tx.Statement.Omits = append(tx.Statement.Omits, "Metadata")
		}
	})

	return db
}

// TestCreateOrganization_AssignsTrialPlan verifies that creating a team
// organization via CreateOrganization automatically creates an active
// OrganizationSubscription with the Trial plan for that org.
func TestCreateOrganization_AssignsTrialPlan(t *testing.T) {
	db := testDBForOrgService(t)

	// Seed a Trial plan
	trialPlan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Trial",
		Description:            "Free trial plan",
		Priority:               0,
		PriceAmount:            0,
		Currency:               "eur",
		BillingInterval:        "month",
		MaxCourses:             5,
		MaxConcurrentTerminals: 1,
		IsActive:               true,
	}
	err := db.Create(trialPlan).Error
	require.NoError(t, err)

	orgService := services.NewOrganizationService(db)
	orgSubService := paymentServices.NewOrganizationSubscriptionService(db)

	userID := uuid.New().String()

	// Create a team organization (no SubscriptionPlanID in input)
	input := dto.CreateOrganizationInput{
		Name:        "test-team-org",
		DisplayName: "Test Team Org",
		Description: "A team org for testing trial auto-assignment",
	}

	org, err := orgService.CreateOrganization(userID, input)
	require.NoError(t, err, "Organization creation should succeed")
	require.NotNil(t, org, "Organization should not be nil")

	// ASSERT: The org should now have an active OrganizationSubscription
	// with the Trial plan.
	sub, err := orgSubService.GetOrganizationSubscription(org.ID)
	assert.NoError(t, err, "Should find an active subscription for the new org")
	assert.NotNil(t, sub, "OrganizationSubscription should exist")

	if sub != nil {
		assert.Equal(t, trialPlan.ID, sub.SubscriptionPlanID,
			"The subscription should be for the Trial plan")
		assert.Equal(t, "active", sub.Status,
			"The subscription should be active (Trial is free)")
		assert.Equal(t, org.ID, sub.OrganizationID,
			"The subscription should belong to the created org")
	}

	// Also verify that the Organization.SubscriptionPlanID field was set
	assert.NotNil(t, org.SubscriptionPlanID,
		"Organization.SubscriptionPlanID should be set after trial assignment")
	if org.SubscriptionPlanID != nil {
		assert.Equal(t, trialPlan.ID, *org.SubscriptionPlanID,
			"Organization.SubscriptionPlanID should point to the Trial plan")
	}
}

// TestCreateOrganization_NoTrialPlan_Succeeds verifies that if no Trial plan
// exists in the database, organization creation still succeeds. The system
// should log a warning but not fail.
func TestCreateOrganization_NoTrialPlan_Succeeds(t *testing.T) {
	db := testDBForOrgService(t)

	// Deliberately do NOT create any Trial plan.
	// The DB is clean (freshTestDB clears all rows).

	orgService := services.NewOrganizationService(db)

	userID := uuid.New().String()

	input := dto.CreateOrganizationInput{
		Name:        "org-without-trial",
		DisplayName: "Org Without Trial",
		Description: "Testing graceful degradation when no Trial plan exists",
	}

	org, err := orgService.CreateOrganization(userID, input)

	// ASSERT: Org creation must still succeed even without a Trial plan
	assert.NoError(t, err, "Org creation should succeed even if no Trial plan exists")
	assert.NotNil(t, org, "Organization should be created")

	if org != nil {
		assert.Equal(t, "org-without-trial", org.Name)
		assert.True(t, org.IsActive)
	}
}

// TestCreateOrganization_ExistingPlan_SkipsTrialAssignment verifies that when
// the CreateOrganizationInput already has a SubscriptionPlanID set, the system
// does NOT override it with the Trial plan.
func TestCreateOrganization_ExistingPlan_SkipsTrialAssignment(t *testing.T) {
	db := testDBForOrgService(t)

	// Create both a Trial plan and a Pro plan
	trialPlan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Trial",
		Description:            "Free trial plan",
		Priority:               0,
		PriceAmount:            0,
		Currency:               "eur",
		BillingInterval:        "month",
		MaxCourses:             5,
		MaxConcurrentTerminals: 1,
		IsActive:               true,
	}
	err := db.Create(trialPlan).Error
	require.NoError(t, err)

	proPlan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Pro",
		Description:            "Pro plan",
		Priority:               20,
		PriceAmount:            1999,
		Currency:               "eur",
		BillingInterval:        "month",
		MaxCourses:             -1,
		MaxConcurrentTerminals: 10,
		IsActive:               true,
	}
	err = db.Create(proPlan).Error
	require.NoError(t, err)

	orgService := services.NewOrganizationService(db)
	orgSubService := paymentServices.NewOrganizationSubscriptionService(db)

	userID := uuid.New().String()

	// Create org with an explicit SubscriptionPlanID (the Pro plan)
	input := dto.CreateOrganizationInput{
		Name:               "pro-team-org",
		DisplayName:        "Pro Team Org",
		Description:        "An org that already has a plan chosen",
		SubscriptionPlanID: &proPlan.ID,
	}

	org, err := orgService.CreateOrganization(userID, input)
	require.NoError(t, err, "Organization creation should succeed")
	require.NotNil(t, org)

	// ASSERT: The subscription created should be for the Pro plan, NOT the Trial.
	// If the feature auto-assigns Trial, it should detect that a plan was already
	// provided and skip the Trial assignment.
	sub, err := orgSubService.GetOrganizationSubscription(org.ID)

	// We expect a subscription to exist (either auto-created for the explicit plan
	// or auto-created as Trial). The key assertion is that if a subscription exists,
	// it should NOT be the Trial plan when a SubscriptionPlanID was explicitly provided.
	if err == nil && sub != nil {
		assert.NotEqual(t, trialPlan.ID, sub.SubscriptionPlanID,
			"Should NOT override the explicit plan with Trial")
		assert.Equal(t, proPlan.ID, sub.SubscriptionPlanID,
			"Subscription should be for the explicitly provided Pro plan")
	}

	// Also check the Organization.SubscriptionPlanID was preserved
	assert.NotNil(t, org.SubscriptionPlanID,
		"Organization should have a SubscriptionPlanID")
	if org.SubscriptionPlanID != nil {
		assert.Equal(t, proPlan.ID, *org.SubscriptionPlanID,
			"Organization.SubscriptionPlanID should remain the Pro plan, not Trial")
	}
}
