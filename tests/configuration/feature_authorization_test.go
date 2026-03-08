package configuration_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authModels "soli/formations/src/auth/models"
	entityRegistration "soli/formations/src/configuration/entityRegistration"
	ems "soli/formations/src/entityManagement/entityManagementService"
)

// registerFeatureAndGetRoles calls the actual RegisterFeature function on a
// fresh EntityRegistrationService and returns the stored roles map.
// The Casbin enforcer is nil during this call (no DB), but roles are still
// recorded in the service's entityRoles map.
func registerFeatureAndGetRoles(t *testing.T) map[string]string {
	t.Helper()

	service := ems.NewEntityRegistrationService()
	entityRegistration.RegisterFeature(service)

	allRoles := service.GetAllEntityRoles()
	featureRoles, exists := allRoles["Feature"]
	require.True(t, exists, "Feature entity must be registered in entityRoles")

	return featureRoles.Roles
}

// TestFeatureRegistration_MemberHasGetOnly verifies that the Feature entity
// registration gives Member only GET access. Feature flags control platform
// behavior and must not be writable by regular users.
func TestFeatureRegistration_MemberHasGetOnly(t *testing.T) {
	roles := registerFeatureAndGetRoles(t)
	memberRole := string(authModels.Member)

	memberMethods, hasMember := roles[memberRole]
	require.True(t, hasMember, "Feature entity must define a role for %q", memberRole)

	// Member should only have GET
	assert.Contains(t, memberMethods, http.MethodGet,
		"Member should have GET access to features")
	assert.NotContains(t, memberMethods, http.MethodPost,
		"Member must NOT have POST access to features (admin-only operation)")
	assert.NotContains(t, memberMethods, http.MethodPatch,
		"Member must NOT have PATCH access to features (admin-only operation)")
	assert.NotContains(t, memberMethods, http.MethodDelete,
		"Member must NOT have DELETE access to features (admin-only operation)")
}

// TestFeatureRegistration_AdminHasFullAccess verifies that the Feature entity
// registration gives Administrator full CRUD access (GET|POST|PATCH|DELETE).
func TestFeatureRegistration_AdminHasFullAccess(t *testing.T) {
	roles := registerFeatureAndGetRoles(t)
	adminRole := string(authModels.Admin)

	adminMethods, hasAdmin := roles[adminRole]
	require.True(t, hasAdmin, "Feature entity must define a role for %q", adminRole)

	assert.Contains(t, adminMethods, http.MethodGet,
		"Admin should have GET access to features")
	assert.Contains(t, adminMethods, http.MethodPost,
		"Admin should have POST access to features")
	assert.Contains(t, adminMethods, http.MethodPatch,
		"Admin should have PATCH access to features")
	assert.Contains(t, adminMethods, http.MethodDelete,
		"Admin should have DELETE access to features")
}
