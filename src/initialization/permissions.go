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
		{"/api/v1/terminals/:id/history", "GET"},
		{"/api/v1/terminals/:id/history", "DELETE"},
		{"/api/v1/terminals/my-history", "DELETE"},
		{"/api/v1/terminals/:id/access-status", "GET"},
		{"/api/v1/terminals/consent-status", "GET"},
		{"/api/v1/terminals/backends", "GET"},
	}

	for _, route := range terminalRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
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
		reconcilePolicy(enforcer, "administrator", route.path, route.method)
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
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Organization terminal session routes - available to all authenticated members
	// (fine-grained org membership checks happen in the controller)
	log.Println("Setting up organization terminal route permissions...")
	reconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/terminal-sessions", "GET")

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
	// (handlers use userId from JWT for self-scoped operations)
	log.Println("Setting up user subscription permissions...")
	subscriptionMemberRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/user-subscriptions/current", "GET"},
		{"/api/v1/user-subscriptions/all", "GET"},
		{"/api/v1/user-subscriptions/usage", "GET"},
		{"/api/v1/user-subscriptions/checkout", "POST"},
		{"/api/v1/user-subscriptions/portal", "POST"},
		{"/api/v1/user-subscriptions/:id/cancel", "POST"},
		{"/api/v1/user-subscriptions/:id/reactivate", "POST"},
		{"/api/v1/user-subscriptions/upgrade", "POST"},
		{"/api/v1/user-subscriptions/usage/check", "POST"},
		{"/api/v1/user-subscriptions/sync-usage-limits", "POST"},
		{"/api/v1/user-subscriptions/purchase-bulk", "POST"},
	}

	for _, route := range subscriptionMemberRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Admin subscription management (handlers have isAdmin() checks as defense-in-depth)
	log.Println("Setting up admin subscription permissions...")
	subscriptionAdminRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/user-subscriptions/analytics", "GET"},
		{"/api/v1/user-subscriptions/admin-assign", "POST"},
		{"/api/v1/user-subscriptions/sync-existing", "POST"},
		{"/api/v1/user-subscriptions/users/:user_id/sync", "POST"},
		{"/api/v1/user-subscriptions/sync-missing-metadata", "POST"},
		{"/api/v1/user-subscriptions/link/:subscription_id", "POST"},
	}

	for _, route := range subscriptionAdminRoutes {
		reconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	// Organization subscription endpoints - available to all authenticated members
	// (handlers check org membership / admin status)
	log.Println("Setting up organization subscription permissions...")
	orgSubscriptionRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/organizations/:id/subscribe", "POST"},
		{"/api/v1/organizations/:id/subscription", "GET"},
		{"/api/v1/organizations/:id/subscription", "DELETE"},
		{"/api/v1/organizations/:id/features", "GET"},
		{"/api/v1/organizations/:id/usage-limits", "GET"},
		{"/api/v1/users/me/features", "GET"},
	}

	for _, route := range orgSubscriptionRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Admin-only organization subscription overview
	reconcilePolicy(enforcer, "administrator", "/api/v1/admin/organizations/subscriptions", "GET")

	// Invoice endpoints
	log.Println("Setting up invoice permissions...")
	reconcilePolicy(enforcer, "member", "/api/v1/invoices/user", "GET")
	reconcilePolicy(enforcer, "member", "/api/v1/invoices/sync", "POST")
	reconcilePolicy(enforcer, "member", "/api/v1/invoices/:id/download", "GET")
	reconcilePolicy(enforcer, "administrator", "/api/v1/invoices/admin/cleanup", "POST")

	// Payment method endpoints
	log.Println("Setting up payment method permissions...")
	reconcilePolicy(enforcer, "member", "/api/v1/payment-methods/user", "GET")
	reconcilePolicy(enforcer, "member", "/api/v1/payment-methods/sync", "POST")
	reconcilePolicy(enforcer, "member", "/api/v1/payment-methods/:id/set-default", "POST")

	// Billing address endpoints
	log.Println("Setting up billing address permissions...")
	reconcilePolicy(enforcer, "member", "/api/v1/billing-addresses/user", "GET")
	reconcilePolicy(enforcer, "member", "/api/v1/billing-addresses/:id/set-default", "POST")

	// Usage metrics endpoints
	log.Println("Setting up usage metrics permissions...")
	reconcilePolicy(enforcer, "member", "/api/v1/usage-metrics/user", "GET")
	reconcilePolicy(enforcer, "member", "/api/v1/usage-metrics/increment", "POST")
	reconcilePolicy(enforcer, "member", "/api/v1/usage-metrics/reset", "POST")

	// Subscription plan sync (admin-only)
	log.Println("Setting up subscription plan admin permissions...")
	reconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/:id/sync-stripe", "POST")
	reconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/sync-stripe", "POST")
	reconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/import-stripe", "POST")

	// Stripe hooks toggle (admin-only)
	reconcilePolicy(enforcer, "administrator", "/api/v1/hooks/stripe/toggle", "POST")

	// Subscription batch endpoints - available to all authenticated members
	log.Println("Setting up subscription batch permissions...")
	batchRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/subscription-batches", "GET"},
		{"/api/v1/subscription-batches/:id", "GET"},
		{"/api/v1/subscription-batches/:id/licenses", "GET"},
		{"/api/v1/subscription-batches/:id/assign", "POST"},
		{"/api/v1/subscription-batches/:id/licenses/:license_id/revoke", "DELETE"},
		{"/api/v1/subscription-batches/:id/quantity", "PATCH"},
		{"/api/v1/subscription-batches/:id/permanent", "DELETE"},
		{"/api/v1/subscription-batches/create-checkout-session", "POST"},
	}

	for _, route := range batchRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	log.Println("✅ Payment and subscription permissions setup completed")
}

// SetupFeedbackPermissions sets up feedback-related permissions
func SetupFeedbackPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up feedback permissions ===")

	reconcilePolicy(enforcer, "member", "/api/v1/feedback/*", "POST")

	log.Println("✅ Feedback permissions setup completed")
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
		{"/api/v1/scenario-sessions/available", "GET"},
		{"/api/v1/scenario-sessions/by-terminal/:terminalId", "GET"},
		{"/api/v1/scenario-sessions/:id/current-step", "GET"},
		{"/api/v1/scenario-sessions/:id/step/:stepOrder", "GET"},
		{"/api/v1/scenario-sessions/:id/verify", "POST"},
		{"/api/v1/scenario-sessions/:id/submit-flag", "POST"},
		{"/api/v1/scenario-sessions/:id/abandon", "POST"},
		{"/api/v1/scenario-sessions/:id/info", "GET"},
		{"/api/v1/scenario-sessions/:id/flags", "GET"},
		{"/api/v1/scenario-sessions/:id/steps/:stepOrder/hints/:level/reveal", "POST"},
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

	// Group-level combined scenario listing (teachers see org + group scenarios)
	reconcilePolicy(enforcer, "member", "/api/v1/groups/:groupId/scenarios", "GET")

	// Organization-level scenario management routes - available to all authenticated members
	// (fine-grained org ownership checks happen in the controller via validateOrgManagerAccess)
	orgScenarioRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/organizations/:id/scenarios", "GET"},
		{"/api/v1/organizations/:id/scenarios/import-json", "POST"},
		{"/api/v1/organizations/:id/scenarios/upload", "POST"},
		{"/api/v1/organizations/:id/scenarios/:scenarioId/export", "GET"},
		{"/api/v1/organizations/:id/scenarios/:scenarioId", "DELETE"},
	}

	for _, route := range orgScenarioRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Admin-only scenario management routes (handlers have isAdmin() checks)
	scenarioAdminRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/scenarios/import", "POST"},
		{"/api/v1/scenarios/seed", "POST"},
		{"/api/v1/scenarios/upload", "POST"},
		{"/api/v1/scenarios/:id/export", "GET"},
		{"/api/v1/scenarios/export", "POST"},
		{"/api/v1/scenarios/import-json", "POST"},
	}

	for _, route := range scenarioAdminRoutes {
		reconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	// Project file routes - available to all authenticated members
	// (files are read-only views of scenario content for rendering)
	projectFileRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/project-files/by-scenario/:scenarioId", "GET"},
		{"/api/v1/project-files/image/:scenarioId/*", "GET"},
		{"/api/v1/project-files/:id/content", "GET"},
		{"/api/v1/project-files/:id/usage", "GET"},
	}

	for _, route := range projectFileRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	log.Println("✅ Scenario and teacher dashboard permissions setup completed")
}

// SetupCoursePermissions sets up course and generation-related permissions
func SetupCoursePermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up course and generation permissions ===")

	courseRoutes := []struct {
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

	for _, route := range courseRoutes {
		reconcilePolicy(enforcer, "member", route.path, route.method)
	}

	log.Println("✅ Course and generation permissions setup completed")
}

// SetupUserManagementPermissions sets up user management and access control permissions
func SetupUserManagementPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up user management permissions ===")

	// User lookup routes - available to members for collaboration (sharing, group management)
	reconcilePolicy(enforcer, "member", "/api/v1/users", "GET")
	reconcilePolicy(enforcer, "member", "/api/v1/users/batch", "POST")
	reconcilePolicy(enforcer, "member", "/api/v1/users/search", "GET")

	// User deletion - admin only
	reconcilePolicy(enforcer, "administrator", "/api/v1/users/:id", "DELETE")

	// Entity access management - admin only (manipulates Casbin RBAC policies directly)
	reconcilePolicy(enforcer, "administrator", "/api/v1/accesses", "POST")
	reconcilePolicy(enforcer, "administrator", "/api/v1/accesses", "DELETE")

	// Entity hook management - admin only
	reconcilePolicy(enforcer, "administrator", "/api/v1/hooks", "GET")
	reconcilePolicy(enforcer, "administrator", "/api/v1/hooks/:hook_name/enable", "POST")
	reconcilePolicy(enforcer, "administrator", "/api/v1/hooks/:hook_name/disable", "POST")

	// Email template testing - admin only
	reconcilePolicy(enforcer, "administrator", "/api/v1/email-templates/:id/test", "POST")

	// WebSSH - available to all authenticated members
	reconcilePolicy(enforcer, "member", "/api/v1/ssh", "GET")

	log.Println("✅ User management permissions setup completed")
}
