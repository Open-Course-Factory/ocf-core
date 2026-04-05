// tests/terminalTrainer/backendForContext_test.go
package terminalTrainer_tests

import (
	"strings"
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

// TestValidateBackendForContext_PlanAllowedBackends_FallsBackToDefault verifies that when
// requesting a backend not in the plan's AllowedBackends (and org has no config),
// the system falls back to the plan's default backend (not a hard rejection).
func TestValidateBackendForContext_PlanAllowedBackends_FallsBackToDefault(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-plan-restricts"
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Org has NO backend config
	org := createOrgWithBackendConfig(t, db, "org-plan-restrict", userID,
		organizationModels.OrgTypeTeam, "", []string{})

	// Plan only allows "shared-pool", default is "shared-pool"
	plan := createPlanWithBackendConfig(t, db, "RestrictedPlan", 5, 2, "shared-pool", []string{"shared-pool"})

	svc := services.NewTerminalTrainerService(db)
	sessionInput := dto.CreateTerminalSessionInput{
		Terms:          "accepted",
		OrganizationID: org.ID.String(),
		Backend:        "premium-pool", // Not in plan's allowed list → falls back to plan default
	}

	// Should NOT reject — falls back to plan default "shared-pool"
	// The error we get is from the actual terminal creation (network), not from backend validation
	_, err = svc.StartSessionWithPlan(userID, sessionInput, plan)
	if err != nil {
		// If there's an error, it should NOT be about backend restriction
		assert.NotContains(t, err.Error(), "not allowed by your subscription plan")
	}
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

// TestValidateBackendForContext_NilOrg_PlanFallsBackToDefault verifies that when there's no
// org context and the requested backend isn't in the plan's allowed list, it falls back to
// the plan's default backend.
func TestValidateBackendForContext_NilOrg_PlanFallsBackToDefault(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-nil-org-plan-restr"
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Plan only allows "shared-pool", default is "shared-pool"
	plan := createPlanWithBackendConfig(t, db, "RestrictedNilOrg", 5, 2, "shared-pool", []string{"shared-pool"})

	svc := services.NewTerminalTrainerService(db)
	sessionInput := dto.CreateTerminalSessionInput{
		Terms:   "accepted",
		Backend: "premium-pool", // Not in plan's allowed list → falls back to plan default
	}

	// Should fall back to plan default, not reject
	_, err = svc.StartSessionWithPlan(userID, sessionInput, plan)
	if err != nil {
		assert.NotContains(t, err.Error(), "not allowed by your subscription plan")
	}
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

	// Plan only allows "shared-pool", default is "shared-pool"
	plan := createPlanWithBackendConfig(t, db, "PersonalRestricted", 5, 2, "shared-pool", []string{"shared-pool"})

	svc := services.NewTerminalTrainerService(db)
	sessionInput := dto.CreateTerminalSessionInput{
		Terms:          "accepted",
		OrganizationID: org.ID.String(),
		Backend:        "premium-pool", // Not in plan's allowed list → falls back to plan default
	}

	// Should fall back to plan default, not reject
	_, err = svc.StartSessionWithPlan(userID, sessionInput, plan)
	if err != nil {
		assert.NotContains(t, err.Error(), "not allowed by your subscription plan")
	}
}

// TestValidateBackendForContext_NilOrg_NoPlanRestrictions_ArbitraryBackend_ShouldRejectOrDefault
// verifies that when a plan has empty AllowedBackends and empty DefaultBackend, an explicit
// backend request should NOT be passed through verbatim. It should either be rejected or
// fall back to the system default.
//
// BUG: The current implementation falls through to `return requestedBackend, nil` at the
// end of validateBackendForContext when plan.AllowedBackends is empty and plan.DefaultBackend
// is empty. This allows any arbitrary backend name to pass validation, which means users
// on unrestricted plans can request backends they shouldn't have access to.
//
// The fix should ensure that when a plan has no backend restrictions (empty AllowedBackends
// and empty DefaultBackend), the system default is returned instead of the arbitrary request.
func TestValidateBackendForContext_NilOrg_NoPlanRestrictions_ArbitraryBackend_ShouldRejectOrDefault(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-no-restrictions-passthrough"
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Plan with NO backend restrictions: empty AllowedBackends and empty DefaultBackend
	plan := createPlanWithBackendConfig(t, db, "UnrestrictedPlan", 5, 2,
		"",         // no DefaultBackend
		[]string{}, // no AllowedBackends
	)

	svc := services.NewTerminalTrainerService(db)
	sessionInput := dto.CreateTerminalSessionInput{
		Terms:   "accepted",
		Backend: "some-arbitrary-backend", // arbitrary backend that doesn't exist
		// No OrganizationID — nil org context
	}

	resp, err := svc.StartSessionWithPlan(userID, sessionInput, plan)

	// EXPECTED behavior: When a plan has no backend configuration (no AllowedBackends,
	// no DefaultBackend), an arbitrary backend request should NOT pass through.
	// The function should return the system default backend, not the arbitrary name.
	//
	// CURRENT BUG: validateBackendForContext returns ("some-arbitrary-backend", nil),
	// passing the arbitrary name through without validation. The function then
	// proceeds to startSession with a possibly non-existent backend name.
	//
	// We verify this bug by checking: if the call succeeds or fails with a network
	// error (connection refused), that proves the arbitrary backend was passed through
	// without validation. If backend validation worked correctly, we'd get either:
	// - An error about the backend not being allowed, OR
	// - A successful session using the system default backend (not the arbitrary one)
	if err != nil {
		// If there's an error, it should be a validation error about the backend,
		// NOT a network/connection error from trying to contact an invalid backend
		assert.NotContains(t, err.Error(), "connection refused",
			"should not reach network call with arbitrary backend — validation should have caught it")
		assert.NotContains(t, err.Error(), "no such host",
			"should not reach network call with arbitrary backend — validation should have caught it")
		assert.NotContains(t, err.Error(), "failed to get user key",
			"should not reach session creation with arbitrary backend — validation should have caught it")
		// The error should be about the backend being rejected
		assert.True(t,
			strings.Contains(err.Error(), "not allowed") || strings.Contains(err.Error(), "invalid backend"),
			"error should indicate backend was rejected by validation, got: %s", err.Error())
	} else {
		// If no error, the session was created — but the backend should be the system
		// default, not the arbitrary one we requested
		assert.NotNil(t, resp)
		assert.NotEqual(t, "some-arbitrary-backend", sessionInput.Backend,
			"arbitrary backend should not have been passed through; should be system default")
	}
}

// TestGetBackendsForContext_OrgHasConfig_UsesOrgBackends verifies that when the org
// has its own backend config, GetBackendsForContext delegates to GetBackendsForOrganization.
func TestGetBackendsForContext_OrgHasConfig_UsesOrgBackends(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-backends-ctx-org"
	org := createOrgWithBackendConfig(t, db, "org-ctx-has-config", userID,
		organizationModels.OrgTypeTeam, "dedicated-1", []string{"dedicated-1"})

	// Add user as member
	db.Create(&organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           "owner",
		IsActive:       true,
	})

	// User has a plan with different backends — but org config should win
	plan := createPlanWithBackendConfig(t, db, "PlanForCtxTest", 10, 5, "shared-pool", []string{"shared-pool"})
	db.Create(&paymentModels.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		SubscriptionType:   "personal",
	})

	svc := services.NewTerminalTrainerService(db)
	// GetBackendsForContext should return org's backends (not plan's)
	// Note: the actual backend IDs won't exist in tt-backend, but the filtering logic
	// is what we're testing. GetBackendsForOrganization will be called, which filters
	// from the cached backend list. Since we can't easily mock tt-backend here,
	// we just verify it doesn't error out for the org-has-config path.
	_, err := svc.GetBackendsForContext(org.ID, userID)
	// May error because tt-backend is not running, but should NOT panic
	if err != nil {
		// Expected: "terminal trainer not configured" or similar network error
		assert.NotContains(t, err.Error(), "not a member")
	}
}

// TestGetBackendsForContext_OrgNoConfig_UsesPlanBackends verifies that when the org
// has no backend config, GetBackendsForContext resolves the user's plan and uses
// plan-level backend filtering.
func TestGetBackendsForContext_OrgNoConfig_UsesPlanBackends(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-backends-ctx-plan"
	org := createOrgWithBackendConfig(t, db, "org-ctx-no-config", userID,
		organizationModels.OrgTypeTeam, "", []string{}) // No backend config

	// Add user as member
	db.Create(&organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           "owner",
		IsActive:       true,
	})

	// User has a plan with specific backends
	plan := createPlanWithBackendConfig(t, db, "PlanWithBackends", 10, 5, "premium-infra", []string{"premium-infra", "free-infra"})
	db.Create(&paymentModels.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		SubscriptionType:   "personal",
	})

	svc := services.NewTerminalTrainerService(db)
	_, err := svc.GetBackendsForContext(org.ID, userID)
	// May error because tt-backend is not running, but the plan resolution path should work
	if err != nil {
		assert.NotContains(t, err.Error(), "not a member")
	}
}

// TestValidateBackendForContext_PlanFallbackToDefault verifies that when a requested
// backend is not in the plan's allowed list but the plan has a default, the system
// falls back to the plan default instead of rejecting.
func TestValidateBackendForContext_PlanFallbackToDefault(t *testing.T) {
	db := freshTestDB(t)

	userID := "user-fallback-default"
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Plan allows only "premium-infra", default is "premium-infra"
	plan := createPlanWithBackendConfig(t, db, "FallbackPlan", 10, 5, "premium-infra", []string{"premium-infra", "free-infra"})

	svc := services.NewTerminalTrainerService(db)
	sessionInput := dto.CreateTerminalSessionInput{
		Terms:   "accepted",
		Backend: "default", // Not in plan's allowed list — should fall back to plan default
	}

	// Should NOT reject with "not allowed" — should fall back to "premium-infra"
	_, err = svc.StartSessionWithPlan(userID, sessionInput, plan)
	if err != nil {
		// Error may come from actual terminal creation (network), but NOT from backend validation
		assert.NotContains(t, err.Error(), "not allowed by your subscription plan")
		assert.NotContains(t, err.Error(), "not allowed")
	}
}
