package authorization_tests

// RED-phase contract for pilot MR-E: migrating POST /email-templates/:id/test
// from a hand-wired custom route (mounted in src/email/routes/routes.go, with a
// Casbin policy + RoutePermission hand-registered in usersRoutes/permissions.go)
// to a declarative ActionConfig on the EmailTemplate entity registration.
//
// These tests DEFINE the post-migration contract. They are RED until the
// EmailTemplate registration declares the "test" action:
//   - GetActions("EmailTemplate") is empty today.
//   - The action-driven Casbin triple / RouteRegistry entry do not exist today
//     (the current entry is registered by usersRoutes, under a different
//     category, and only when that registration runs — not from the email
//     registration in isolation).
//
// The MR-B action framework (ActionConfig, GetActions, SetEntityActionAccesses,
// ResourceBasePath) already exists on this stacked branch, so the file compiles;
// the assertions are what fail.
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
	userController "soli/formations/src/auth/routes/usersRoutes"
	registration "soli/formations/src/email/entityRegistration"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

const emailTestActionPath = "/api/v1/email-templates/:id/test"

// stubGlobalEnforcerForEmail swaps the global casdoor.Enforcer (used by
// RegisterTypedEntity's setDefaultEntityAccesses AND SetEntityActionAccesses) for
// a capturing mock, and returns it so the driven Casbin policies can be asserted.
func stubGlobalEnforcerForEmail(t *testing.T) *mocks.MockEnforcer {
	t.Helper()
	mock := mocks.NewMockEnforcer()
	orig := casdoor.Enforcer
	casdoor.Enforcer = mock
	t.Cleanup(func() { casdoor.Enforcer = orig })
	return mock
}

// ---------------------------------------------------------------------------
// 1. The EmailTemplate registration carries the "test" action.
//
//    After booting the registration (RegisterEmailTemplate against a fresh
//    service), GetActions("EmailTemplate") must contain exactly one action:
//    Name "test", POST, item scope, administrator role, admin_only access,
//    non-nil handler.
// ---------------------------------------------------------------------------

func TestEmailTemplateRegistration_DeclaresTestAction(t *testing.T) {
	stubGlobalEnforcerForEmail(t)
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	service := ems.NewEntityRegistrationService()
	registration.RegisterEmailTemplate(service)

	actions := service.GetActions("EmailTemplate")
	require.Len(t, actions, 1, "EmailTemplate must declare exactly one custom action (test)")

	a := actions[0]
	assert.Equal(t, "test", a.Name)
	assert.Equal(t, "POST", a.Method)
	assert.Equal(t, entityManagementInterfaces.ActionScopeItem, a.Scope, "test targets a single template → item scope")
	assert.Equal(t, access.RoleAdministrator, a.Role, "test-email is admin-only at Layer 1")
	assert.Equal(t, access.AdminOnly, a.Access.Type, "test-email is admin-only at Layer 2")
	assert.NotNil(t, a.Handler, "action must carry a non-nil handler factory")
}

// ---------------------------------------------------------------------------
// 2. Layer 1: booting the registration registers the exact Casbin triple for
//    the action route — subject = administrator (the action's Role), path =
//    /api/v1/email-templates/:id/test, method = POST.
// ---------------------------------------------------------------------------

func TestEmailTemplateRegistration_RegistersLayer1AdminPolicy(t *testing.T) {
	mock := stubGlobalEnforcerForEmail(t)
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	service := ems.NewEntityRegistrationService()
	registration.RegisterEmailTemplate(service)

	ps := collectPolicies(mock)
	assertPolicy(t, ps, access.RoleAdministrator, emailTestActionPath, "POST")
}

// ---------------------------------------------------------------------------
// 3. Layer 2: the action's RoutePermission is declared in the RouteRegistry,
//    keyed by POST:path, carrying AdminOnly access under the "EmailTemplate"
//    category (RegisterEnforced uses the entity name as the category).
// ---------------------------------------------------------------------------

func TestEmailTemplateRegistration_RegistersLayer2AdminOnlyRoutePermission(t *testing.T) {
	stubGlobalEnforcerForEmail(t)
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	service := ems.NewEntityRegistrationService()
	registration.RegisterEmailTemplate(service)

	perm, found := access.RouteRegistry.Lookup("POST", emailTestActionPath)
	require.True(t, found, "expected a RoutePermission for the email-test action route")
	assert.Equal(t, access.RoleAdministrator, perm.Role)
	assert.Equal(t, access.AdminOnly, perm.Access.Type)
	assert.Equal(t, "EmailTemplate", perm.Category, "action RoutePermission is categorised under the entity name")
}

// ---------------------------------------------------------------------------
// 4. Old wiring GONE: the usersRoutes permission registration must no longer own
//    the email-test route. After running ONLY RegisterUserPermissions against a
//    reset RouteRegistry, the email-test path must MISS — proving the entry was
//    removed from usersRoutes/permissions.go and now lives solely on the entity
//    registration.
//
//    This is RED today (usersRoutes still registers the entry, so Lookup hits)
//    and turns GREEN once the dev deletes that hand-wired entry.
// ---------------------------------------------------------------------------

func TestUserPermissions_NoLongerOwnsEmailTestRoute(t *testing.T) {
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	mock := mocks.NewMockEnforcer()
	userController.RegisterUserPermissions(mock)

	_, found := access.RouteRegistry.Lookup("POST", emailTestActionPath)
	assert.False(t, found,
		"usersRoutes must no longer register the email-test route; it belongs to the EmailTemplate action registration")

	ps := collectPolicies(mock)
	assert.False(t, ps.has(access.RoleAdministrator, emailTestActionPath, "POST"),
		"usersRoutes must no longer register the email-test Casbin policy")
}
