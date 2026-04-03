// tests/terminalTrainer/backendForContext_test.go
package terminalTrainer_tests

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	organizationModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// createPlanWithBackendConfig creates a subscription plan with backend configuration.
func createPlanWithBackendConfig(
	t *testing.T, db *gorm.DB,
	name string, priority int, maxTerminals int,
	defaultBackend string, allowedBackends []string,
) *paymentModels.SubscriptionPlan {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:                  entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                       name,
		Priority:                   priority,
		PriceAmount:                0,
		Currency:                   "eur",
		BillingInterval:            "month",
		IsActive:                   true,
		MaxConcurrentTerminals:     maxTerminals,
		MaxCourses:                 5,
		MaxSessionDurationMinutes:  60,
		AllowedMachineSizes:        []string{"all"},
		DefaultBackend:             defaultBackend,
		AllowedBackends:            allowedBackends,
	}
	err := db.Create(plan).Error
	assert.NoError(t, err)
	return plan
}

// createOrgWithBackendConfig creates an org with optional backend config.
func createOrgWithBackendConfig(
	t *testing.T, db *gorm.DB,
	orgName string, ownerID string,
	orgType organizationModels.OrganizationType,
	defaultBackend string, allowedBackends []string,
) *organizationModels.Organization {
	t.Helper()
	org := &organizationModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             orgName,
		DisplayName:      orgName,
		OwnerUserID:      ownerID,
		IsActive:         true,
		OrganizationType: orgType,
		IsPersonal:       orgType == organizationModels.OrgTypePersonal,
		MaxGroups:        10,
		MaxMembers:       50,
		DefaultBackend:   defaultBackend,
		AllowedBackends:  allowedBackends,
	}
	err := db.Omit("Metadata").Create(org).Error
	assert.NoError(t, err)
	return org
}

// TestValidateBackendForContext_OrgHasBackendConfig_RejectsDisallowed verifies that
// org-level restrictions block backends not in the org's AllowedBackends,
// even when the plan allows them.
func TestValidateBackendForContext_OrgHasBackendConfig_RejectsDisallowed(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-org-rejects"
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Org only allows "cloud1"
	org := createOrgWithBackendConfig(t, db, "org-restrict", userID,
		organizationModels.OrgTypeTeam, "cloud1", []string{"cloud1"})

	// Plan allows both "local" and "cloud1"
	plan := createPlanWithBackendConfig(t, db, "PlanAny", 10, 5, "local", []string{"local", "cloud1"})

	svc := services.NewTerminalTrainerService(db)
	sessionInput := dto.CreateTerminalSessionInput{
		Terms:          "accepted",
		OrganizationID: org.ID.String(),
		Backend:        "local", // Requesting backend NOT in org's allowed list
	}

	// This should fail with org restriction error (validateBackendForOrg rejects it)
	resp, err := svc.StartSessionWithPlan(userID, sessionInput, plan)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "not allowed for your organization")
}

// TestValidateBackendForContext_PlanAllowedBackends_Restricts verifies that when
// requesting a backend not in the plan's AllowedBackends (and org has no config),
// the request is rejected.
func TestValidateBackendForContext_PlanAllowedBackends_Restricts(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-plan-restricts"
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Org has NO backend config
	org := createOrgWithBackendConfig(t, db, "org-plan-restrict", userID,
		organizationModels.OrgTypeTeam, "", []string{})

	// Plan only allows "shared-pool"
	plan := createPlanWithBackendConfig(t, db, "RestrictedPlan", 5, 2, "shared-pool", []string{"shared-pool"})

	svc := services.NewTerminalTrainerService(db)
	sessionInput := dto.CreateTerminalSessionInput{
		Terms:          "accepted",
		OrganizationID: org.ID.String(),
		Backend:        "premium-pool", // Not in plan's allowed list
	}

	resp, err := svc.StartSessionWithPlan(userID, sessionInput, plan)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "not allowed by your subscription plan")
}

// TestValidateBackendForContext_OrgHasConfig_PlanRestrictionIgnored verifies that
// when org has its own backend config, the plan's AllowedBackends is NOT checked.
// Instead, the org's rules take precedence.
func TestValidateBackendForContext_OrgHasConfig_PlanRestrictionIgnored(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-org-wins"
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Org allows "premium-pool" but NOT "shared-pool"
	org := createOrgWithBackendConfig(t, db, "org-premium", userID,
		organizationModels.OrgTypeTeam, "premium-pool", []string{"premium-pool"})

	// Plan only allows "shared-pool" — but org config takes precedence
	plan := createPlanWithBackendConfig(t, db, "FreePlan", 0, 1, "shared-pool", []string{"shared-pool"})

	svc := services.NewTerminalTrainerService(db)
	sessionInput := dto.CreateTerminalSessionInput{
		Terms:          "accepted",
		OrganizationID: org.ID.String(),
		Backend:        "shared-pool", // In plan's allowed list, but NOT in org's
	}

	// Should fail with ORG restriction (not plan restriction)
	resp, err := svc.StartSessionWithPlan(userID, sessionInput, plan)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "not allowed for your organization")
}

// TestValidateBackendForContext_NilOrg_PlanRestricts verifies that when there's no
// org context, plan-level restrictions still apply.
func TestValidateBackendForContext_NilOrg_PlanRestricts(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-nil-org-plan-restr"
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Plan only allows "shared-pool"
	plan := createPlanWithBackendConfig(t, db, "RestrictedNilOrg", 5, 2, "shared-pool", []string{"shared-pool"})

	svc := services.NewTerminalTrainerService(db)
	sessionInput := dto.CreateTerminalSessionInput{
		Terms:   "accepted",
		Backend: "premium-pool", // Not in plan's allowed list, no org context
	}

	resp, err := svc.StartSessionWithPlan(userID, sessionInput, plan)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "not allowed by your subscription plan")
}

// TestValidateBackendForContext_PersonalOrg_NoConfig_PlanRestricts verifies that
// a personal org (never has backend config) falls through to plan-level rules.
func TestValidateBackendForContext_PersonalOrg_NoConfig_PlanRestricts(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-personal-plan-restr"
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Personal org (no backend config)
	org := createOrgWithBackendConfig(t, db, "personal-org", userID,
		organizationModels.OrgTypePersonal, "", []string{})

	// Plan only allows "shared-pool"
	plan := createPlanWithBackendConfig(t, db, "PersonalRestricted", 5, 2, "shared-pool", []string{"shared-pool"})

	svc := services.NewTerminalTrainerService(db)
	sessionInput := dto.CreateTerminalSessionInput{
		Terms:          "accepted",
		OrganizationID: org.ID.String(),
		Backend:        "premium-pool", // Not in plan's allowed list
	}

	resp, err := svc.StartSessionWithPlan(userID, sessionInput, plan)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "not allowed by your subscription plan")
}
