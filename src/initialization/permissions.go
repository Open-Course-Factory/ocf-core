package initialization

import (
	"log"
	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
)

// reconcilePolicy compares the desired policy with what exists in the DB and only
// makes changes when they differ. This is safe for production — no blind deletes.
func reconcilePolicy(enforcer interfaces.EnforcerInterface, role, path, method string) {
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
		reconcilePolicy(enforcer, role, "/api/v1/users/:id", "GET")
		reconcilePolicy(enforcer, role, "/api/v1/users/me/*", "(GET|POST|PATCH|DELETE)")

		// Auth endpoints
		reconcilePolicy(enforcer, role, "/api/v1/auth/permissions", "GET")
		reconcilePolicy(enforcer, role, "/api/v1/auth/me", "GET")
		reconcilePolicy(enforcer, role, "/api/v1/auth/verify-status", "GET")
	}

	log.Println("✅ Authentication and user permissions setup completed")
}

// SetupTerminalPermissions sets up terminal-related permissions
func SetupTerminalPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up terminal permissions ===")

	// User Terminal Key custom routes - available to all authenticated members
	log.Println("Setting up user terminal key custom route permissions...")
	reconcilePolicy(enforcer, "member", "/api/v1/user-terminal-keys/regenerate", "POST")
	reconcilePolicy(enforcer, "member", "/api/v1/user-terminal-keys/my-key", "GET")

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
	}

	for _, route := range terminalRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Incus UI proxy routes - available to all authenticated members
	// (fine-grained backend access checks happen in IsUserAuthorizedForBackend)
	log.Println("Setting up Incus UI proxy permissions...")
	reconcilePolicy(enforcer, "member", "/api/v1/incus-ui/:backendId/*", "(GET|POST|PUT|PATCH|DELETE)")

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
		reconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	log.Println("✅ Security admin permissions setup completed")
}

// SetupPaymentPermissions sets up payment and subscription-related permissions
func SetupPaymentPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up payment and subscription permissions ===")

	// User subscription endpoints - available to all authenticated members
	log.Println("Setting up user subscription permissions...")
	reconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/current", "GET")
	reconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/portal", "POST")
	reconcilePolicy(enforcer, "member", "/api/v1/invoices/user", "GET")
	reconcilePolicy(enforcer, "member", "/api/v1/payment-methods/user", "GET")

	// Subscription batch endpoints - available to all authenticated members
	log.Println("Setting up subscription batch permissions...")
	batchRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/subscription-batches", "GET"},                                    // List accessible batches
		{"/api/v1/subscription-batches/:id", "GET"},                                // Get batch details
		{"/api/v1/subscription-batches/:id/licenses", "GET"},                       // List licenses in batch
		{"/api/v1/subscription-batches/:id/assign", "POST"},                        // Assign a license
		{"/api/v1/subscription-batches/:id/licenses/:license_id/revoke", "DELETE"}, // Revoke a license
		{"/api/v1/subscription-batches/:id/quantity", "PATCH"},                     // Update quantity
		{"/api/v1/subscription-batches/:id/permanent", "DELETE"},                   // Permanently delete batch
		{"/api/v1/subscription-batches/create-checkout-session", "POST"},           // Create checkout session
	}

	for _, route := range batchRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	log.Println("✅ Payment and subscription permissions setup completed")
}

// SetupScenarioPermissions sets up scenario session and teacher dashboard permissions
func SetupScenarioPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up scenario and teacher dashboard permissions ===")

	// Scenario session routes - available to all authenticated members
	sessionRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/scenario-sessions/start", "POST"},
		{"/api/v1/scenario-sessions/my", "GET"},
		{"/api/v1/scenario-sessions/by-terminal/:terminalId", "GET"},
		{"/api/v1/scenario-sessions/:id/current-step", "GET"},
		{"/api/v1/scenario-sessions/:id/step/:stepOrder", "GET"},
		{"/api/v1/scenario-sessions/:id/verify", "POST"},
		{"/api/v1/scenario-sessions/:id/submit-flag", "POST"},
		{"/api/v1/scenario-sessions/:id/abandon", "POST"},
	}

	for _, route := range sessionRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Teacher dashboard routes - available to all authenticated members
	// (fine-grained group ownership checks happen in the controller)
	teacherRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/teacher/groups/:groupId/activity", "GET"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/results", "GET"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/analytics", "GET"},
		{"/api/v1/teacher/groups/:groupId/sessions/:sessionId/detail", "GET"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/bulk-start", "POST"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/reset-sessions", "POST"},
	}

	for _, route := range teacherRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Group-level scenario import/export routes - available to all authenticated members
	// (fine-grained group ownership checks happen in the controller via validateTeacherAccess)
	groupScenarioRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/groups/:groupId/scenarios/upload", "POST"},
		{"/api/v1/groups/:groupId/scenarios/import-json", "POST"},
		{"/api/v1/groups/:groupId/scenarios/:scenarioId/export", "GET"},
	}

	for _, route := range groupScenarioRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Admin-only scenario management routes
	reconcilePolicy(enforcer, "administrator", "/api/v1/scenarios/import", "POST")
	reconcilePolicy(enforcer, "administrator", "/api/v1/scenarios/seed", "POST")

	log.Println("✅ Scenario and teacher dashboard permissions setup completed")
}
