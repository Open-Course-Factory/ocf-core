package auth_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authModels "soli/formations/src/auth/models"
	registration "soli/formations/src/auth/entityRegistration"
	ems "soli/formations/src/entityManagement/entityManagementService"
)

// TestSshKeyRegistration_MemberHasGetOnly verifies that the Member role
// only has GET access to the SshKey entity (no POST, PATCH, DELETE).
func TestSshKeyRegistration_MemberHasGetOnly(t *testing.T) {
	service := ems.NewEntityRegistrationService()
	registration.RegisterSshKey(service)

	allRoles := service.GetAllEntityRoles()
	sshKeyRoles, ok := allRoles["SshKey"]
	require.True(t, ok, "SshKey entity should be registered")

	memberMethods, hasMember := sshKeyRoles.Roles[string(authModels.Member)]
	require.True(t, hasMember, "Member role should be defined for SshKey")

	// Member should have GET only
	assert.Contains(t, memberMethods, http.MethodGet,
		"Member should have GET access to SshKey")
	assert.NotContains(t, memberMethods, http.MethodPost,
		"Member should NOT have POST access to SshKey")
	assert.NotContains(t, memberMethods, http.MethodPatch,
		"Member should NOT have PATCH access to SshKey")
	assert.NotContains(t, memberMethods, http.MethodDelete,
		"Member should NOT have DELETE access to SshKey")
}

// TestSshKeyRegistration_AdminHasFullAccess verifies that the Administrator role
// has full CRUD access (GET, POST, PATCH, DELETE) to the SshKey entity.
func TestSshKeyRegistration_AdminHasFullAccess(t *testing.T) {
	service := ems.NewEntityRegistrationService()
	registration.RegisterSshKey(service)

	allRoles := service.GetAllEntityRoles()
	sshKeyRoles, ok := allRoles["SshKey"]
	require.True(t, ok, "SshKey entity should be registered")

	adminMethods, hasAdmin := sshKeyRoles.Roles[string(authModels.Admin)]
	require.True(t, hasAdmin, "Administrator role should be defined for SshKey")

	assert.Contains(t, adminMethods, http.MethodGet,
		"Administrator should have GET access to SshKey")
	assert.Contains(t, adminMethods, http.MethodPost,
		"Administrator should have POST access to SshKey")
	assert.Contains(t, adminMethods, http.MethodPatch,
		"Administrator should have PATCH access to SshKey")
	assert.Contains(t, adminMethods, http.MethodDelete,
		"Administrator should have DELETE access to SshKey")
}
