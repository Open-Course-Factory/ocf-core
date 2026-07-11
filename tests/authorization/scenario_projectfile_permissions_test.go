package authorization_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/mocks"
	scenarioController "soli/formations/src/scenarios/routes"
)

// TestRegisterScenarioPermissions_ProjectFileAdminRoutesGrantAdministrator is the
// RED test for #408. Two project-file routes are declared Layer-2 AdminOnly but
// their Layer-1 Casbin grant is still "member":
//
//	GET /api/v1/project-files/by-scenario/:scenarioId   (list files for a scenario)
//	GET /api/v1/project-files/:id/usage                 (file usage info)
//
// The handlers self-enforce isProjectFileAdmin, so this is not a live bug, but the
// Layer-1 grant should match the AdminOnly intent and deny non-admins at the
// gateway (defense-in-depth), consistent with the sibling admin-only scenario
// routes (import/seed/upload/duplicate). The fix flips Role from member →
// administrator on these two routes only.
//
// RED today: both are registered for "member".
// After the fix: both are registered for "administrator".
func TestRegisterScenarioPermissions_ProjectFileAdminRoutesGrantAdministrator(t *testing.T) {
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

	roleFor := func(path, method string) (string, bool) {
		for _, p := range addedPolicies {
			if p.path == path && p.method == method {
				return p.role, true
			}
		}
		return "", false
	}

	// The two admin-only project-file routes MUST be Layer-1 gated to
	// "administrator", not "member".
	adminRoutes := []struct {
		path   string
		method string
		desc   string
	}{
		{
			path:   "/api/v1/project-files/by-scenario/:scenarioId",
			method: "GET",
			desc:   "list project files for a scenario (admin only)",
		},
		{
			path:   "/api/v1/project-files/:id/usage",
			method: "GET",
			desc:   "project file usage info (admin only)",
		},
	}

	for _, route := range adminRoutes {
		t.Run(route.desc, func(t *testing.T) {
			role, found := roleFor(route.path, route.method)
			require.True(t, found,
				"RegisterScenarioPermissions must register a Casbin policy for %s %s",
				route.method, route.path)
			assert.Equal(t, "administrator", role,
				"%s %s is Layer-2 AdminOnly — its Layer-1 grant must be \"administrator\", "+
					"not %q, so non-admins are denied at the gateway (defense-in-depth)",
				route.method, route.path, role)
		})
	}
}

// TestRegisterScenarioPermissions_ProjectFileMemberFacingRoutesStayMember is the
// over-tightening guard for #408. The member-facing project-file routes (content
// + image) are Layer-2 Public and MUST remain Layer-1 "member" so learners can
// still fetch scenario file content and images. This guards against the #408 fix
// accidentally flipping these to administrator along with the two admin routes.
func TestRegisterScenarioPermissions_ProjectFileMemberFacingRoutesStayMember(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	scenarioController.RegisterScenarioPermissions(mockEnforcer)

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

	roleFor := func(path, method string) (string, bool) {
		for _, p := range addedPolicies {
			if p.path == path && p.method == method {
				return p.role, true
			}
		}
		return "", false
	}

	memberRoutes := []struct {
		path   string
		method string
		desc   string
	}{
		{
			path:   "/api/v1/project-files/:id/content",
			method: "GET",
			desc:   "project file content (member-facing, Public)",
		},
		{
			path:   "/api/v1/project-files/image/:scenarioId/*",
			method: "GET",
			desc:   "scenario image (member-facing, Public)",
		},
	}

	for _, route := range memberRoutes {
		t.Run(route.desc, func(t *testing.T) {
			role, found := roleFor(route.path, route.method)
			require.True(t, found,
				"RegisterScenarioPermissions must register a Casbin policy for %s %s",
				route.method, route.path)
			assert.Equal(t, "member", role,
				"%s %s is member-facing (Public) — it must stay Layer-1 \"member\"; "+
					"the #408 admin-tightening must NOT touch it (got %q)",
				route.method, route.path, role)
		})
	}
}
