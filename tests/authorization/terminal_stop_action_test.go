package authorization_tests

// RED-phase contract for pilot MR-F: migrating POST /terminals/:id/stop from a
// hand-wired custom route (mounted in src/terminalTrainer/routes/terminalRoutes.go
// with a Casbin policy + RoutePermission hand-registered in the module's
// permissions.go) to a declarative ActionConfig on the Terminal entity
// registration — WITH a custom middleware gate.
//
// This pilot is the sibling of MR-E (email test verb) but proves the extra
// Middlewares field: the stop action must carry exactly one middleware factory
// (the RequireTerminalAccess ownership gate) so the declarative action reproduces
// the current chain
//   AuthManagement -> RequireTerminalAccess -> StopSession
// The route generator already mounts action.Middlewares between the auth
// middleware and the handler (src/entityManagement/swagger/routeGenerator.go),
// so declaring the factory in Middlewares is all the migration needs.
//
// These tests DEFINE the post-migration contract. They are RED until the Terminal
// registration declares the "stop" action:
//   - GetActions("Terminal") is empty today.
//   - The action-driven Casbin triple / RouteRegistry entry do not exist today
//     from the registration in isolation (the current entry is registered by the
//     terminal module's RegisterTerminalPermissions, under the "Terminals"
//     category, and only when that registration runs).
//
// The MR-B action framework (ActionConfig with the Middlewares field, GetActions,
// SetEntityActionAccesses, ResourceBasePath) already exists on this stacked
// branch, so the file compiles; the assertions are what fail.
//
// Helpers collectPolicies / assertPolicy / policySet live in all_permissions_test.go
// (same package) and are reused here.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	registration "soli/formations/src/terminalTrainer/entityRegistration"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

const terminalStopActionPath = "/api/v1/terminals/:id/stop"

// stubGlobalEnforcerForTerminal swaps the global casdoor.Enforcer (used by
// RegisterTypedEntity's setDefaultEntityAccesses AND SetEntityActionAccesses) for
// a capturing mock, and returns it so the driven Casbin policies can be asserted.
func stubGlobalEnforcerForTerminal(t *testing.T) *mocks.MockEnforcer {
	t.Helper()
	mock := mocks.NewMockEnforcer()
	orig := casdoor.Enforcer
	casdoor.Enforcer = mock
	t.Cleanup(func() { casdoor.Enforcer = orig })
	return mock
}

// ---------------------------------------------------------------------------
// 1. The Terminal registration carries the "stop" action WITH its ownership
//    middleware.
//
//    After booting the registration (RegisterTerminal against a fresh service),
//    GetActions("Terminal") must contain exactly one action: Name "stop", POST,
//    item scope, member role, self_scoped access, non-nil handler, and exactly
//    one middleware factory (the RequireTerminalAccess gate). The Middlewares
//    length is the distinguishing contract of this pilot.
// ---------------------------------------------------------------------------

func TestTerminalRegistration_DeclaresStopAction(t *testing.T) {
	stubGlobalEnforcerForTerminal(t)
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	service := ems.NewEntityRegistrationService()
	registration.RegisterTerminal(service)

	actions := service.GetActions("Terminal")
	require.Len(t, actions, 1, "Terminal must declare exactly one custom action (stop)")

	a := actions[0]
	assert.Equal(t, "stop", a.Name)
	assert.Equal(t, "POST", a.Method)
	assert.Equal(t, entityManagementInterfaces.ActionScopeItem, a.Scope, "stop targets a single terminal → item scope")
	assert.Equal(t, access.RoleMember, a.Role, "stop is a member action at Layer 1 (all real users are members)")
	assert.Equal(t, access.SelfScoped, a.Access.Type, "stop keeps SelfScoped: the RequireTerminalAccess gate is the authoritative ownership check")
	assert.NotNil(t, a.Handler, "action must carry a non-nil handler factory")
	require.Len(t, a.Middlewares, 1, "stop must carry exactly one middleware factory: the RequireTerminalAccess ownership gate")
	assert.NotNil(t, a.Middlewares[0], "the RequireTerminalAccess middleware factory must be non-nil")
}

// ---------------------------------------------------------------------------
// 2. Layer 1: booting the registration registers the exact Casbin triple for the
//    action route — subject = member (the action's Role), path =
//    /api/v1/terminals/:id/stop, method = POST.
// ---------------------------------------------------------------------------

func TestTerminalRegistration_RegistersLayer1MemberPolicy(t *testing.T) {
	mock := stubGlobalEnforcerForTerminal(t)
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	service := ems.NewEntityRegistrationService()
	registration.RegisterTerminal(service)

	ps := collectPolicies(mock)
	assertPolicy(t, ps, access.RoleMember, terminalStopActionPath, "POST")
}

// ---------------------------------------------------------------------------
// 3. Layer 2: the action's RoutePermission is declared in the RouteRegistry,
//    keyed by POST:path, carrying SelfScoped access under the "Terminal"
//    category (SetEntityActionAccesses uses the entity name as the category).
// ---------------------------------------------------------------------------

func TestTerminalRegistration_RegistersLayer2SelfScopedRoutePermission(t *testing.T) {
	stubGlobalEnforcerForTerminal(t)
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	service := ems.NewEntityRegistrationService()
	registration.RegisterTerminal(service)

	perm, found := access.RouteRegistry.Lookup("POST", terminalStopActionPath)
	require.True(t, found, "expected a RoutePermission for the terminal-stop action route")
	assert.Equal(t, access.RoleMember, perm.Role)
	assert.Equal(t, access.SelfScoped, perm.Access.Type)
	assert.Equal(t, "Terminal", perm.Category, "action RoutePermission is categorised under the entity name")
}

// ---------------------------------------------------------------------------
// 4. Old wiring GONE: the terminal module's RegisterTerminalPermissions must no
//    longer own the stop route. After running ONLY RegisterTerminalPermissions
//    against a reset RouteRegistry, the stop path must MISS (both the Layer 2
//    RoutePermission and the Layer 1 policy) — proving the entry was removed from
//    the module permissions.go and now lives solely on the entity registration.
//
//    This is RED today (RegisterTerminalPermissions still registers the entry, so
//    Lookup hits) and turns GREEN once the dev deletes that hand-wired entry.
// ---------------------------------------------------------------------------

func TestTerminalPermissions_NoLongerOwnsStopRoute(t *testing.T) {
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	mock := mocks.NewMockEnforcer()
	terminalController.RegisterTerminalPermissions(mock)

	_, found := access.RouteRegistry.Lookup("POST", terminalStopActionPath)
	assert.False(t, found,
		"RegisterTerminalPermissions must no longer register the stop route; it belongs to the Terminal action registration")

	ps := collectPolicies(mock)
	assert.False(t, ps.has(access.RoleMember, terminalStopActionPath, "POST"),
		"RegisterTerminalPermissions must no longer register the stop Casbin policy")
}
