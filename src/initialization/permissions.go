package initialization

import (
	"log"
	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
	"strings"
)

// SetupPaymentRolePermissions sets up role-based permissions for payment system
func SetupPaymentRolePermissions(enforcer interfaces.EnforcerInterface) {
	enforcer.LoadPolicy()

	// Add permissions for /api/v1/users/:id (which includes /api/v1/users/me)
	// Get all Casdoor roles that map to the "member" OCF role
	casdoorRoles := authModels.GetCasdoorRolesForOCFRole(authModels.Member)

	for _, role := range casdoorRoles {
		// Add permission for /api/v1/users/:id (the actual route pattern)
		_, err1 := enforcer.AddPolicy(role, "/api/v1/users/:id", "GET")
		if err1 != nil {
			if strings.Contains(err1.Error(), "UNIQUE") {
				log.Printf("Permission already exists: %s /api/v1/users/:id", role)
			} else {
				log.Printf("Error adding permission for %s /api/v1/users/:id: %v", role, err1)
			}
		} else {
			log.Printf("✅ Added %s permission for /api/v1/users/:id (includes /me)", role)
		}

		// Add permission for /api/v1/users/me/* sub-paths
		_, err2 := enforcer.AddPolicy(role, "/api/v1/users/me/*", "(GET|POST|PATCH|DELETE)")
		if err2 != nil {
			if strings.Contains(err2.Error(), "UNIQUE") {
				log.Printf("Permission already exists: %s /api/v1/users/me/*", role)
			} else {
				log.Printf("Error adding permission for %s /api/v1/users/me/*: %v", role, err2)
			}
		} else {
			log.Printf("✅ Added %s permission for /api/v1/users/me/*", role)
		}

		// Add permission for /api/v1/auth/permissions (new permissions endpoint)
		_, err3 := enforcer.AddPolicy(role, "/api/v1/auth/permissions", "GET")
		if err3 != nil {
			if strings.Contains(err3.Error(), "UNIQUE") {
				log.Printf("Permission already exists: %s /api/v1/auth/permissions", role)
			} else {
				log.Printf("Error adding permission for %s /api/v1/auth/permissions: %v", role, err3)
			}
		} else {
			log.Printf("✅ Added %s permission for /api/v1/auth/permissions", role)
		}
	}

	// Terminal and subscription permissions for all members
	// These are now handled by entity registration (terminals) and subscription logic
	// Keeping minimal overrides here for subscription-related endpoints

	// Note: Standard terminal CRUD permissions are automatically set via TerminalRegistration entity
	// But custom terminal routes need explicit permissions

	// User Terminal Key custom routes - available to all authenticated members
	log.Println("Setting up user terminal key custom route permissions...")
	_, err := enforcer.AddPolicy("member", "/api/v1/user-terminal-keys/regenerate", "POST")
	if err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		log.Printf("Error adding regenerate permission: %v", err)
	} else {
		log.Printf("✅ Added member permission for /api/v1/user-terminal-keys/regenerate")
	}

	_, err = enforcer.AddPolicy("member", "/api/v1/user-terminal-keys/my-key", "GET")
	if err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		log.Printf("Error adding my-key permission: %v", err)
	} else {
		log.Printf("✅ Added member permission for /api/v1/user-terminal-keys/my-key")
	}

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
		_, err := enforcer.AddPolicy("member", route.path, route.method)
		if err != nil && !strings.Contains(err.Error(), "UNIQUE") {
			log.Printf("Error adding terminal permission %s %s: %v", route.method, route.path, err)
		} else {
			log.Printf("✅ Added member permission for %s %s", route.method, route.path)
		}
	}

	// User subscription endpoints - available to all authenticated members
	enforcer.AddPolicy("member", "/api/v1/user-subscriptions/current", "GET")
	enforcer.AddPolicy("member", "/api/v1/user-subscriptions/portal", "POST")
	enforcer.AddPolicy("member", "/api/v1/invoices/user", "GET")
	enforcer.AddPolicy("member", "/api/v1/payment-methods/user", "GET")

	log.Printf("✅ Terminal permissions setup completed")
}
