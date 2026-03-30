package authorization_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/mocks"
	scenarioController "soli/formations/src/scenarios/routes"
)

// TestSetupScenarioPermissions_GroupLevelRoutes verifies that RegisterScenarioPermissions
// registers Casbin policies for the group-level scenario routes (upload, import-json,
// export). Without these policies, all users (including group owners) get 403 errors
// on these endpoints.
func TestSetupScenarioPermissions_GroupLevelRoutes(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	scenarioController.RegisterScenarioPermissions(mockEnforcer)

	// Collect all (role, path, method) tuples that were added as policies.
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

	// These 3 group-level routes MUST be registered for the "member" role.
	// They are currently missing from SetupScenarioPermissions, causing 403 for all users.
	requiredGroupRoutes := []struct {
		path   string
		method string
		desc   string
	}{
		{
			path:   "/api/v1/groups/:groupId/scenarios/upload",
			method: "POST",
			desc:   "scenario upload via group",
		},
		{
			path:   "/api/v1/groups/:groupId/scenarios/import-json",
			method: "POST",
			desc:   "scenario JSON import via group",
		},
		{
			path:   "/api/v1/groups/:groupId/scenarios/:scenarioId/export",
			method: "GET",
			desc:   "scenario export via group",
		},
	}

	require.NotEmpty(t, addedPolicies, "RegisterScenarioPermissions should add at least some policies")

	for _, route := range requiredGroupRoutes {
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
					"without it, all users get 403 on this endpoint",
				route.method, route.path)
		})
	}
}
