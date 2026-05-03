package authorization_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/mocks"
	scenarioController "soli/formations/src/scenarios/routes"
)

// TestRegisterScenarioPermissions_PlatformExportRoutesAllowMember verifies that
// RegisterScenarioPermissions registers Casbin policies for the PLATFORM-LEVEL
// scenario export routes (single + bulk) under the "member" role. Today these
// two routes are registered for "administrator" only, which means an org-manager
// who can otherwise create/edit/delete a scenario receives a 403
// "Administrator role required" the moment they try to export from the editor.
//
// The fix moves the two export routes out of the admin-only block in
// src/scenarios/routes/permissions.go so member is a valid Layer-1 role; the
// fine-grained "is this user allowed to export THIS scenario?" check is then
// enforced inside the controller via canManageScenario (with admin bypass).
func TestRegisterScenarioPermissions_PlatformExportRoutesAllowMember(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	scenarioController.RegisterScenarioPermissions(mockEnforcer)

	// Collect every (role, path, method) tuple that was registered.
	type policy struct {
		role   string
		path   string
		method string
	}

	var addedPolicies []policy
	for _, call := range mockEnforcer.AddPolicyCalls {
		if len(call) >= 3 {
			role, _ := call[0].(string)
			path, _ := call[1].(string)
			method, _ := call[2].(string)
			addedPolicies = append(addedPolicies, policy{role: role, path: path, method: method})
		}
	}

	require.NotEmpty(t, addedPolicies, "RegisterScenarioPermissions should add at least some policies")

	// Both platform-level export routes MUST be reachable by "member".
	requiredPlatformExportRoutes := []struct {
		path   string
		method string
		desc   string
	}{
		{
			path:   "/api/v1/scenarios/:id/export",
			method: "GET",
			desc:   "platform-level single scenario export",
		},
		{
			path:   "/api/v1/scenarios/export",
			method: "POST",
			desc:   "platform-level bulk scenario export",
		},
	}

	for _, route := range requiredPlatformExportRoutes {
		t.Run(route.desc, func(t *testing.T) {
			found := false
			for _, p := range addedPolicies {
				if p.role == "member" && p.path == route.path && p.method == route.method {
					found = true
					break
				}
			}
			assert.True(t, found,
				"RegisterScenarioPermissions must register member policy for %s %s — "+
					"without it, org managers and scenario creators get 403 "+
					"\"Administrator role required\" on this endpoint",
				route.method, route.path)
		})
	}
}
