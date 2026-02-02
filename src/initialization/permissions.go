package initialization

import (
	"log"
	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
	"strings"
)

// addPolicyWithErrorHandling is a helper function to add policies with consistent error handling
func addPolicyWithErrorHandling(enforcer interfaces.EnforcerInterface, role, path, method string) {
	_, err := enforcer.AddPolicy(role, path, method)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			log.Printf("Permission already exists: %s %s %s", role, path, method)
		} else {
			log.Printf("Error adding permission for %s %s %s: %v", role, path, method, err)
		}
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
		addPolicyWithErrorHandling(enforcer, role, "/api/v1/users/:id", "GET")
		addPolicyWithErrorHandling(enforcer, role, "/api/v1/users/me/*", "(GET|POST|PATCH|DELETE)")

		// Auth endpoints
		addPolicyWithErrorHandling(enforcer, role, "/api/v1/auth/permissions", "GET")
		addPolicyWithErrorHandling(enforcer, role, "/api/v1/auth/me", "GET")
		addPolicyWithErrorHandling(enforcer, role, "/api/v1/auth/verify-status", "GET")
	}

	log.Println("✅ Authentication and user permissions setup completed")
}

// SetupTerminalPermissions sets up terminal-related permissions
func SetupTerminalPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up terminal permissions ===")

	// User Terminal Key custom routes - available to all authenticated members
	log.Println("Setting up user terminal key custom route permissions...")
	addPolicyWithErrorHandling(enforcer, "member", "/api/v1/user-terminal-keys/regenerate", "POST")
	addPolicyWithErrorHandling(enforcer, "member", "/api/v1/user-terminal-keys/my-key", "GET")

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
		addPolicyWithErrorHandling(enforcer, "member", route.path, route.method)
	}

	log.Println("✅ Terminal permissions setup completed")
}

// SetupPaymentPermissions sets up payment and subscription-related permissions
func SetupPaymentPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up payment and subscription permissions ===")

	// User subscription endpoints - available to all authenticated members
	log.Println("Setting up user subscription permissions...")
	addPolicyWithErrorHandling(enforcer, "member", "/api/v1/user-subscriptions/current", "GET")
	addPolicyWithErrorHandling(enforcer, "member", "/api/v1/user-subscriptions/portal", "POST")
	addPolicyWithErrorHandling(enforcer, "member", "/api/v1/invoices/user", "GET")
	addPolicyWithErrorHandling(enforcer, "member", "/api/v1/payment-methods/user", "GET")

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
		addPolicyWithErrorHandling(enforcer, "member", route.path, route.method)
	}

	log.Println("✅ Payment and subscription permissions setup completed")
}
