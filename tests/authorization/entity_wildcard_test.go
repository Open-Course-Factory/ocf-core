package authorization_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/mocks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

// TestEntityRegistration_UsesIdNotWildcard verifies that entity registration
// creates Casbin policies with /:id (single segment) instead of /* (all sub-paths).
//
// The /* wildcard was a security issue: for entities with custom sub-routes
// (like organizations), the wildcard policy granted member access to ALL
// sub-paths (e.g., /organizations/:id/import which should be admin-only).
func TestEntityRegistration_UsesIdNotWildcard(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()
	service := ems.NewEntityRegistrationService()

	roles := entityManagementInterfaces.EntityRoles{
		Roles: map[string]string{
			"member":        "(GET)",
			"administrator": "(GET|POST|PATCH|DELETE)",
		},
	}

	service.SetDefaultEntityAccesses("TestEntity", roles, mockEnforcer)

	// Collect all policies that were added
	var paths []string
	for _, call := range mockEnforcer.AddPolicyCalls {
		if len(call) >= 2 {
			path, _ := call[1].(string)
			paths = append(paths, path)
		}
	}

	require.NotEmpty(t, paths, "should have added policies")

	// Verify NO policy uses /* wildcard
	for _, path := range paths {
		assert.NotContains(t, path, "/*",
			"entity policy should use /:id not /* — wildcard matches all sub-paths and bypasses custom route restrictions")
	}

	// Verify policies use /:id for resource access
	found := false
	for _, path := range paths {
		if path == "/api/v1/test-entities/:id" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have a policy for /api/v1/test-entities/:id")

	// Verify list path exists without parameter
	foundList := false
	for _, path := range paths {
		if path == "/api/v1/test-entities" {
			foundList = true
			break
		}
	}
	assert.True(t, foundList, "should have a policy for /api/v1/test-entities (list)")
}
