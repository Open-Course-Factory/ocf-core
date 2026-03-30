package initialization

import (
	"log"
	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
)

// ReconcilePolicy compares the desired policy with what exists in the DB and only
// makes changes when they differ. This is safe for production — no blind deletes.
// Exported so module-level route packages can register their own permissions.
func ReconcilePolicy(enforcer interfaces.EnforcerInterface, role, path, method string) {
	existing, err := enforcer.GetFilteredPolicy(0, role, path)
	if err != nil {
		log.Printf("Error reading policy for %s %s: %v — falling back to add", role, path, err)
		enforcer.AddPolicy(role, path, method)
		return
	}

	// Check if exact policy already exists
	for _, policy := range existing {
		if len(policy) >= 3 && policy[2] == method {
			return // Already correct, nothing to do
		}
	}

	// Policy is missing or has a different method — fix it
	if len(existing) > 0 {
		oldMethod := existing[0][2]
		enforcer.RemoveFilteredPolicy(0, role, path)
		log.Printf("🔄 Updating permission %s %s: %s → %s", role, path, oldMethod, method)
	}

	_, err = enforcer.AddPolicy(role, path, method)
	if err != nil {
		log.Printf("Error adding permission for %s %s %s: %v", role, path, method, err)
	} else {
		log.Printf("✅ Added %s permission for %s %s", role, method, path)
	}
}

// SetupAuthPermissions sets up authentication and user-related permissions
func SetupAuthPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up authentication and user permissions ===")

	// Get all Casdoor roles that map to the "member" OCF role
	casdoorRoles := authModels.GetCasdoorRolesForOCFRole(authModels.Member)

	for _, role := range casdoorRoles {
		// User endpoints
		ReconcilePolicy(enforcer, role, "/api/v1/users/:id", "GET")
		ReconcilePolicy(enforcer, role, "/api/v1/users/me/*", "(GET|POST|PATCH|DELETE)")

		// Auth endpoints
		ReconcilePolicy(enforcer, role, "/api/v1/auth/permissions", "GET")
		ReconcilePolicy(enforcer, role, "/api/v1/auth/me", "GET")
		ReconcilePolicy(enforcer, role, "/api/v1/auth/verify-status", "GET")
	}

	log.Println("✅ Authentication and user permissions setup completed")
}

// SetupTerminalPermissions sets up terminal-related permissions
func SetupTerminalPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up terminal permissions ===")

	// User Terminal Key custom routes - available to all authenticated members
	log.Println("Setting up user terminal key custom route permissions...")
	ReconcilePolicy(enforcer, "member", "/api/v1/user-terminal-keys/regenerate", "POST")
	ReconcilePolicy(enforcer, "member", "/api/v1/user-terminal-keys/my-key", "GET")

	// Terminal custom routes - available to all authenticated members
	log.Println("Setting up terminal custom route permissions...")
	terminalRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/terminals/start-session", "POST"},
		{"/api/v1/terminals/user-sessions", "GET"},
		{"/api/v1/terminals/shared-with-me", "GET"},
		{"/api/v1/terminals/sync-all", "POST"},
		{"/api/v1/terminals/instance-types", "GET"},
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
		{"/api/v1/terminals/:id/access-status", "GET"},
		{"/api/v1/terminals/consent-status", "GET"},
		{"/api/v1/terminals/backends", "GET"},
	}

	for _, route := range terminalRoutes {
		ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Terminal admin routes
	log.Println("Setting up terminal admin route permissions...")
	terminalAdminRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/terminals/backends/:backendId/set-default", "PATCH"},
		{"/api/v1/terminals/enums/status", "GET"},
		{"/api/v1/terminals/enums/refresh", "POST"},
		{"/api/v1/terminals/fix-hide-permissions", "POST"},
	}

	for _, route := range terminalAdminRoutes {
		ReconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	// Group terminal routes - available to all authenticated members
	// (fine-grained group ownership checks happen in the controller)
	log.Println("Setting up group terminal route permissions...")
	groupTerminalRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/class-groups/:id/bulk-create-terminals", "POST"},
		{"/api/v1/class-groups/:id/command-history", "GET"},
		{"/api/v1/class-groups/:id/command-history-stats", "GET"},
	}

	for _, route := range groupTerminalRoutes {
		ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Organization terminal session routes - available to all authenticated members
	// (fine-grained org membership checks happen in the controller)
	log.Println("Setting up organization terminal route permissions...")
	ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/terminal-sessions", "GET")

	// Incus UI proxy routes - available to all authenticated members
	// (fine-grained backend access checks happen in IsUserAuthorizedForBackend)
	log.Println("Setting up Incus UI proxy permissions...")
	ReconcilePolicy(enforcer, "member", "/api/v1/incus-ui/:backendId/*", "(GET|POST|PUT|PATCH|DELETE)")

	log.Println("✅ Terminal permissions setup completed")
}

// SetupSecurityAdminPermissions sets up security admin panel permissions (admin-only)
func SetupSecurityAdminPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up security admin permissions ===")

	adminRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/admin/security/policies", "GET"},
		{"/api/v1/admin/security/user-permissions", "GET"},
		{"/api/v1/admin/security/entity-roles", "GET"},
		{"/api/v1/admin/security/health-checks", "GET"},
	}

	for _, route := range adminRoutes {
		ReconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	log.Println("✅ Security admin permissions setup completed")
}

// SetupPaymentPermissions is kept for backward compatibility but delegates
// to the payment module's own RegisterPaymentPermissions. This function
// will be removed once all callers are updated.
// NOTE: Now a no-op — permissions are registered by paymentController.RegisterPaymentPermissions
func SetupPaymentPermissions(enforcer interfaces.EnforcerInterface) {
	// Permissions moved to src/payment/routes/permissions.go
}

// SetupFeedbackPermissions sets up feedback-related permissions
func SetupFeedbackPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up feedback permissions ===")

	ReconcilePolicy(enforcer, "member", "/api/v1/feedback/*", "POST")

	log.Println("✅ Feedback permissions setup completed")
}

// SetupScenarioPermissions is kept for backward compatibility but is now a no-op.
// Permissions are registered by scenarioController.RegisterScenarioPermissions.
func SetupScenarioPermissions(enforcer interfaces.EnforcerInterface) {
	// Permissions moved to src/scenarios/routes/permissions.go
}

// SetupCoursePermissions is kept for backward compatibility but is now a no-op.
// Permissions are registered by courseController.RegisterCoursePermissions.
func SetupCoursePermissions(enforcer interfaces.EnforcerInterface) {
	// Permissions moved to src/courses/routes/courseRoutes/permissions.go
}

// SetupUserManagementPermissions is kept for backward compatibility but is now a no-op.
// Permissions are registered by userController.RegisterUserPermissions.
func SetupUserManagementPermissions(enforcer interfaces.EnforcerInterface) {
	// Permissions moved to src/auth/routes/usersRoutes/permissions.go
}
