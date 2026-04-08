package authorization_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	securityAdminController "soli/formations/src/auth/routes/securityAdminRoutes"
	userController "soli/formations/src/auth/routes/usersRoutes"
	courseController "soli/formations/src/courses/routes/courseRoutes"
	organizationRoutes "soli/formations/src/organizations/routes"
	paymentController "soli/formations/src/payment/routes"
	scenarioController "soli/formations/src/scenarios/routes"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// policySet collects all policies added by a Setup function for easy assertion.
type policySet struct {
	policies []policy
}

type policy struct {
	role   string
	path   string
	method string
}

func collectPolicies(mock *mocks.MockEnforcer) policySet {
	var ps policySet
	for _, call := range mock.AddPolicyCalls {
		if len(call) >= 3 {
			role, _ := call[0].(string)
			path, _ := call[1].(string)
			method, _ := call[2].(string)
			ps.policies = append(ps.policies, policy{role: role, path: path, method: method})
		}
	}
	return ps
}

func (ps policySet) has(role, path, method string) bool {
	for _, p := range ps.policies {
		if p.role == role && p.path == path && p.method == method {
			return true
		}
	}
	return false
}

func assertPolicy(t *testing.T, ps policySet, role, path, method string) {
	t.Helper()
	assert.True(t, ps.has(role, path, method),
		"missing Casbin policy: role=%q path=%q method=%q", role, path, method)
}

// ---------------------------------------------------------------------------
// Auth permissions
// ---------------------------------------------------------------------------

func TestSetupAuthPermissions_ExistingPolicies(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	userController.RegisterAuthPermissions(mock)
	ps := collectPolicies(mock)

	// Users/:id GET should be registered for each Casdoor role that maps to member
	// At minimum, the "member" equivalent roles should get these:
	expectedPaths := []struct {
		path   string
		method string
	}{
		{"/api/v1/users/:id", "GET"},
		{"/api/v1/users/me/*", "(GET|POST|PATCH|DELETE)"},
		{"/api/v1/auth/permissions", "GET"},
		{"/api/v1/auth/me", "GET"},
		{"/api/v1/auth/verify-status", "GET"},
	}

	for _, ep := range expectedPaths {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			found := false
			for _, p := range ps.policies {
				if p.path == ep.path && p.method == ep.method {
					found = true
					break
				}
			}
			assert.True(t, found, "missing auth policy for %s %s", ep.method, ep.path)
		})
	}
}

// ---------------------------------------------------------------------------
// Terminal permissions (existing + newly added in #171)
// ---------------------------------------------------------------------------

func TestSetupTerminalPermissions_MemberRoutes(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	terminalController.RegisterTerminalPermissions(mock)
	ps := collectPolicies(mock)

	memberRoutes := []struct {
		path   string
		method string
	}{
		// Existing (pre-#171)
		{"/api/v1/terminals/user-sessions", "GET"},
		{"/api/v1/terminals/shared-with-me", "GET"},
		{"/api/v1/terminals/sync-all", "POST"},
		{"/api/v1/terminals/metrics", "GET"},
		{"/api/v1/terminals/:id/console", "GET"},
		{"/api/v1/terminals/:id/stop", "POST"},
		{"/api/v1/terminals/:id/share", "POST"},
		{"/api/v1/terminals/:id/share/:user_id", "DELETE"},
		{"/api/v1/terminals/:id/shares", "GET"},
		{"/api/v1/terminals/:id/info", "GET"},
		{"/api/v1/terminals/:id/hide", "POST"},
		{"/api/v1/terminals/:id/hide", "DELETE"},
		{"/api/v1/terminals/:id/sync", "POST"},
		{"/api/v1/terminals/:id/status", "GET"},
		{"/api/v1/terminals/:id/history", "GET"},
		{"/api/v1/terminals/:id/history", "DELETE"},
		{"/api/v1/terminals/my-history", "DELETE"},
		// Added in #171
		{"/api/v1/terminals/:id/access-status", "GET"},
		{"/api/v1/terminals/consent-status", "GET"},
		{"/api/v1/terminals/backends", "GET"},
		// Catalog proxy routes (cached from tt-backend)
		{"/api/v1/terminals/catalog-sizes", "GET"},
		{"/api/v1/terminals/catalog-features", "GET"},
		// User terminal keys
		{"/api/v1/user-terminal-keys/regenerate", "POST"},
		{"/api/v1/user-terminal-keys/my-key", "GET"},
		// Group terminal routes
		{"/api/v1/class-groups/:id/bulk-create-terminals", "POST"},
		{"/api/v1/class-groups/:id/command-history", "GET"},
		{"/api/v1/class-groups/:id/command-history-stats", "GET"},
		// Organization terminal routes
		{"/api/v1/organizations/:id/terminal-sessions", "GET"},
		// Incus UI proxy
		{"/api/v1/incus-ui/:backendId/*", "(GET|POST|PUT|PATCH|DELETE)"},
	}

	for _, r := range memberRoutes {
		t.Run("member "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "member", r.path, r.method)
		})
	}
}

func TestSetupTerminalPermissions_AdminRoutes(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	terminalController.RegisterTerminalPermissions(mock)
	ps := collectPolicies(mock)

	adminRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/terminals/backends/:backendId/set-default", "PATCH"},
		{"/api/v1/terminals/enums/status", "GET"},
		{"/api/v1/terminals/enums/refresh", "POST"},
		{"/api/v1/terminals/fix-hide-permissions", "POST"},
	}

	for _, r := range adminRoutes {
		t.Run("admin "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "administrator", r.path, r.method)
		})
	}
}

// ---------------------------------------------------------------------------
// Security admin permissions (existing)
// ---------------------------------------------------------------------------

func TestSetupSecurityAdminPermissions(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	securityAdminController.RegisterSecurityAdminPermissions(mock)
	ps := collectPolicies(mock)

	routes := []struct {
		path   string
		method string
	}{
		{"/api/v1/admin/security/policies", "GET"},
		{"/api/v1/admin/security/user-permissions", "GET"},
		{"/api/v1/admin/security/entity-roles", "GET"},
		{"/api/v1/admin/security/health-checks", "GET"},
	}

	for _, r := range routes {
		t.Run("admin "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "administrator", r.path, r.method)
		})
	}
}

// ---------------------------------------------------------------------------
// Payment permissions (existing + all missing routes)
// ---------------------------------------------------------------------------

func TestSetupPaymentPermissions_MemberRoutes(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	paymentController.RegisterPaymentPermissions(mock)
	ps := collectPolicies(mock)

	memberRoutes := []struct {
		path   string
		method string
	}{
		// Existing
		{"/api/v1/user-subscriptions/current", "GET"},
		{"/api/v1/user-subscriptions/portal", "POST"},
		{"/api/v1/invoices/user", "GET"},
		{"/api/v1/payment-methods/user", "GET"},
		// Subscription batches (existing)
		{"/api/v1/subscription-batches", "GET"},
		{"/api/v1/subscription-batches/:id", "GET"},
		{"/api/v1/subscription-batches/:id/licenses", "GET"},
		{"/api/v1/subscription-batches/:id/assign", "POST"},
		{"/api/v1/subscription-batches/:id/licenses/:license_id/revoke", "DELETE"},
		{"/api/v1/subscription-batches/:id/quantity", "PATCH"},
		{"/api/v1/subscription-batches/:id/permanent", "DELETE"},
		{"/api/v1/subscription-batches/create-checkout-session", "POST"},
		// NEW: User subscription routes
		{"/api/v1/user-subscriptions/all", "GET"},
		{"/api/v1/user-subscriptions/usage", "GET"},
		{"/api/v1/user-subscriptions/checkout", "POST"},
		{"/api/v1/user-subscriptions/:id/cancel", "POST"},
		{"/api/v1/user-subscriptions/:id/reactivate", "POST"},
		{"/api/v1/user-subscriptions/upgrade", "POST"},
		{"/api/v1/user-subscriptions/usage/check", "POST"},
		{"/api/v1/user-subscriptions/sync-usage-limits", "POST"},
		{"/api/v1/user-subscriptions/purchase-bulk", "POST"},
		// NEW: Organization subscription routes
		{"/api/v1/organizations/:id/subscribe", "POST"},
		{"/api/v1/organizations/:id/subscription", "GET"},
		{"/api/v1/organizations/:id/subscription", "DELETE"},
		{"/api/v1/organizations/:id/features", "GET"},
		{"/api/v1/organizations/:id/usage-limits", "GET"},
		{"/api/v1/users/me/features", "GET"},
		// NEW: Invoice routes
		{"/api/v1/invoices/sync", "POST"},
		{"/api/v1/invoices/:id/download", "GET"},
		// NEW: Payment method routes
		{"/api/v1/payment-methods/sync", "POST"},
		{"/api/v1/payment-methods/:id/set-default", "POST"},
		// NEW: Billing address routes
		{"/api/v1/billing-addresses/user", "GET"},
		{"/api/v1/billing-addresses/:id/set-default", "POST"},
		// NEW: Usage metrics routes (read-only for members)
		{"/api/v1/usage-metrics/user", "GET"},
	}

	for _, r := range memberRoutes {
		t.Run("member "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "member", r.path, r.method)
		})
	}
}

func TestSetupPaymentPermissions_AdminRoutes(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	paymentController.RegisterPaymentPermissions(mock)
	ps := collectPolicies(mock)

	adminRoutes := []struct {
		path   string
		method string
	}{
		// NEW: Admin subscription routes
		{"/api/v1/user-subscriptions/analytics", "GET"},
		{"/api/v1/user-subscriptions/admin-assign", "POST"},
		{"/api/v1/user-subscriptions/sync-existing", "POST"},
		{"/api/v1/user-subscriptions/users/:user_id/sync", "POST"},
		{"/api/v1/user-subscriptions/sync-missing-metadata", "POST"},
		{"/api/v1/user-subscriptions/link/:subscription_id", "POST"},
		// NEW: Admin org subscription overview
		{"/api/v1/admin/organizations/subscriptions", "GET"},
		// NEW: Admin invoice cleanup
		{"/api/v1/invoices/admin/cleanup", "POST"},
		// NEW: Subscription plan sync (admin-only)
		{"/api/v1/subscription-plans/:id/sync-stripe", "POST"},
		{"/api/v1/subscription-plans/sync-stripe", "POST"},
		{"/api/v1/subscription-plans/import-stripe", "POST"},
		// NEW: Stripe hooks toggle
		{"/api/v1/hooks/stripe/toggle", "POST"},
		// NEW: Usage metrics admin routes (prevent subscription limit bypass)
		{"/api/v1/usage-metrics/increment", "POST"},
		{"/api/v1/usage-metrics/reset", "POST"},
	}

	for _, r := range adminRoutes {
		t.Run("admin "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "administrator", r.path, r.method)
		})
	}
}

// ---------------------------------------------------------------------------
// Feedback permissions (existing)
// ---------------------------------------------------------------------------

func TestSetupFeedbackPermissions(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	userController.RegisterFeedbackPermissions(mock)
	ps := collectPolicies(mock)

	assertPolicy(t, ps, "member", "/api/v1/feedback/*", "POST")
}

// ---------------------------------------------------------------------------
// Scenario permissions (existing + new routes)
// ---------------------------------------------------------------------------

func TestSetupScenarioPermissions_MemberRoutes(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	scenarioController.RegisterScenarioPermissions(mock)
	ps := collectPolicies(mock)

	memberRoutes := []struct {
		path   string
		method string
	}{
		// Session routes (existing)
		{"/api/v1/scenario-sessions/start", "POST"},
		{"/api/v1/scenario-sessions/my", "GET"},
		{"/api/v1/scenario-sessions/by-terminal/:terminalId", "GET"},
		{"/api/v1/scenario-sessions/:id/current-step", "GET"},
		{"/api/v1/scenario-sessions/:id/step/:stepOrder", "GET"},
		{"/api/v1/scenario-sessions/:id/verify", "POST"},
		{"/api/v1/scenario-sessions/:id/submit-flag", "POST"},
		{"/api/v1/scenario-sessions/:id/abandon", "POST"},
		// NEW: Missing session routes
		{"/api/v1/scenario-sessions/available", "GET"},
		{"/api/v1/scenario-sessions/:id/info", "GET"},
		{"/api/v1/scenario-sessions/:id/flags", "GET"},
		{"/api/v1/scenario-sessions/:id/steps/:stepOrder/hints/:level/reveal", "POST"},
		// Teacher dashboard (existing)
		{"/api/v1/teacher/groups/:groupId/activity", "GET"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/results", "GET"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/analytics", "GET"},
		{"/api/v1/teacher/groups/:groupId/sessions/:sessionId/detail", "GET"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/bulk-start", "POST"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/reset-sessions", "POST"},
		// Group scenario routes (existing)
		{"/api/v1/groups/:groupId/scenarios/upload", "POST"},
		{"/api/v1/groups/:groupId/scenarios/import-json", "POST"},
		{"/api/v1/groups/:groupId/scenarios/:scenarioId/export", "GET"},
		{"/api/v1/groups/:groupId/scenarios", "GET"},
		// Org scenario routes (existing)
		{"/api/v1/organizations/:id/scenarios", "GET"},
		{"/api/v1/organizations/:id/scenarios/import-json", "POST"},
		{"/api/v1/organizations/:id/scenarios/upload", "POST"},
		{"/api/v1/organizations/:id/scenarios/:scenarioId/export", "GET"},
		{"/api/v1/organizations/:id/scenarios/:scenarioId", "DELETE"},
		{"/api/v1/organizations/:id/scenarios/:scenarioId/duplicate", "POST"},
		// Preview route
		{"/api/v1/scenarios/:id/preview", "POST"},
		// NEW: Project file routes
		{"/api/v1/project-files/by-scenario/:scenarioId", "GET"},
		{"/api/v1/project-files/image/:scenarioId/*", "GET"},
		{"/api/v1/project-files/:id/content", "GET"},
		{"/api/v1/project-files/:id/usage", "GET"},
	}

	for _, r := range memberRoutes {
		t.Run("member "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "member", r.path, r.method)
		})
	}
}

func TestSetupScenarioPermissions_AdminRoutes(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	scenarioController.RegisterScenarioPermissions(mock)
	ps := collectPolicies(mock)

	adminRoutes := []struct {
		path   string
		method string
	}{
		// Existing
		{"/api/v1/scenarios/import", "POST"},
		{"/api/v1/scenarios/seed", "POST"},
		// NEW: Admin scenario management
		{"/api/v1/scenarios/upload", "POST"},
		{"/api/v1/scenarios/:id/export", "GET"},
		{"/api/v1/scenarios/export", "POST"},
		{"/api/v1/scenarios/import-json", "POST"},
		{"/api/v1/scenarios/:id/duplicate", "POST"},
	}

	for _, r := range adminRoutes {
		t.Run("admin "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "administrator", r.path, r.method)
		})
	}
}

// ---------------------------------------------------------------------------
// Course permissions (ALL NEW — no Setup function exists yet)
// ---------------------------------------------------------------------------

func TestSetupCoursePermissions_MemberRoutes(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	courseController.RegisterCoursePermissions(mock)
	ps := collectPolicies(mock)

	memberRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/courses/git", "POST"},
		{"/api/v1/courses/source", "POST"},
		{"/api/v1/courses/generate", "POST"},
		{"/api/v1/courses/versions", "GET"},
		{"/api/v1/courses/by-version", "GET"},
		{"/api/v1/generations/:id/status", "GET"},
		{"/api/v1/generations/:id/download", "GET"},
		{"/api/v1/generations/:id/retry", "POST"},
		{"/api/v1/generations", "GET"},
		{"/api/v1/generations", "POST"},
		{"/api/v1/generations/:id", "DELETE"},
	}

	for _, r := range memberRoutes {
		t.Run("member "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "member", r.path, r.method)
		})
	}
}

// ---------------------------------------------------------------------------
// User management permissions (ALL NEW — no Setup function exists yet)
// ---------------------------------------------------------------------------

func TestSetupUserManagementPermissions_MemberRoutes(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	userController.RegisterUserPermissions(mock)
	ps := collectPolicies(mock)

	memberRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/users", "GET"},
		{"/api/v1/users/batch", "POST"},
		{"/api/v1/users/search", "GET"},
		{"/api/v1/ssh", "GET"},
	}

	for _, r := range memberRoutes {
		t.Run("member "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "member", r.path, r.method)
		})
	}
}

func TestSetupUserManagementPermissions_AdminRoutes(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	userController.RegisterUserPermissions(mock)
	ps := collectPolicies(mock)

	adminRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/users/:id", "DELETE"},
		// CRITICAL: accesses routes must be admin-only (manipulate RBAC policies)
		{"/api/v1/accesses", "POST"},
		{"/api/v1/accesses", "DELETE"},
		// Hook management
		{"/api/v1/hooks", "GET"},
		{"/api/v1/hooks/:hook_name/enable", "POST"},
		{"/api/v1/hooks/:hook_name/disable", "POST"},
		// Email template testing
		{"/api/v1/email-templates/:id/test", "POST"},
	}

	for _, r := range adminRoutes {
		t.Run("admin "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "administrator", r.path, r.method)
		})
	}
}

// ---------------------------------------------------------------------------
// Organization permissions (custom routes — migrated from organizationRoutes.go)
// ---------------------------------------------------------------------------

func TestRegisterOrganizationPermissions_MemberRoutes(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	organizationRoutes.RegisterOrganizationPermissions(mock)
	ps := collectPolicies(mock)

	memberRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/organizations/:id/members", "GET"},
		{"/api/v1/organizations/:id/groups", "GET"},
		{"/api/v1/organizations/:id/convert-to-team", "POST"},
		{"/api/v1/organizations/:id/backends", "GET"},
		{"/api/v1/organizations/:id/import", "POST"},
		{"/api/v1/organizations/:id/groups/:groupId/regenerate-passwords", "POST"},
	}

	for _, r := range memberRoutes {
		t.Run("member "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "member", r.path, r.method)
		})
	}
}

func TestRegisterOrganizationPermissions_AdminRoutes(t *testing.T) {
	mock := mocks.NewMockEnforcer()
	organizationRoutes.RegisterOrganizationPermissions(mock)
	ps := collectPolicies(mock)

	adminRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/organizations/:id/backends", "PUT"},
	}

	for _, r := range adminRoutes {
		t.Run("admin "+r.method+" "+r.path, func(t *testing.T) {
			assertPolicy(t, ps, "administrator", r.path, r.method)
		})
	}
}

// ---------------------------------------------------------------------------
// Entity CRUD permission registration in RouteRegistry (Phase 3.5)
// ---------------------------------------------------------------------------

func TestRouteRegistry_RegisterEntity(t *testing.T) {
	// Reset registry to isolate this test
	access.RouteRegistry.Reset()

	entity := access.EntityCRUDPermissions{
		Entity: "TestWidget",
		Create: access.AccessRule{Type: access.SelfScoped},
		Read:   access.AccessRule{Type: access.Public},
		Update: access.AccessRule{Type: access.EntityOwner, Entity: "TestWidget", Field: "UserID"},
		Delete: access.AccessRule{Type: access.AdminOnly},
	}

	// RegisterEntity does not exist yet — this should fail to compile
	access.RouteRegistry.RegisterEntity(entity)

	ref := access.RouteRegistry.GetReference()
	assert.Len(t, ref.Entities, 1, "expected 1 entity in the registry after RegisterEntity")
	assert.Equal(t, "TestWidget", ref.Entities[0].Entity)
}

func TestEntityRegistration_PopulatesRegistry(t *testing.T) {
	// Reset registry to isolate this test
	access.RouteRegistry.Reset()

	entity := access.EntityCRUDPermissions{
		Entity: "TestWidget",
		Create: access.AccessRule{Type: access.SelfScoped},
		Read:   access.AccessRule{Type: access.Public},
		Update: access.AccessRule{Type: access.EntityOwner, Entity: "TestWidget", Field: "UserID"},
		Delete: access.AccessRule{Type: access.AdminOnly},
	}

	// RegisterEntity does not exist yet — this should fail to compile
	access.RouteRegistry.RegisterEntity(entity)

	ref := access.RouteRegistry.GetReference()

	// Verify the entity is present
	if assert.Len(t, ref.Entities, 1, "expected 1 registered entity") {
		registered := ref.Entities[0]

		// Verify entity name
		assert.Equal(t, "TestWidget", registered.Entity)

		// Verify all 4 CRUD AccessRules
		assert.Equal(t, access.SelfScoped, registered.Create.Type, "Create rule type")
		assert.Equal(t, access.Public, registered.Read.Type, "Read rule type")

		assert.Equal(t, access.EntityOwner, registered.Update.Type, "Update rule type")
		assert.Equal(t, "TestWidget", registered.Update.Entity, "Update rule entity")
		assert.Equal(t, "UserID", registered.Update.Field, "Update rule field")

		assert.Equal(t, access.AdminOnly, registered.Delete.Type, "Delete rule type")
	}
}
