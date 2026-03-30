package userController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	"soli/formations/src/initialization"
)

// RegisterUserPermissions registers Casbin permissions for user management,
// access control, entity hooks, email templates, and SSH routes.
func RegisterUserPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up user management and access control permissions ===")

	// --- Member routes ---

	// User lookup routes
	memberRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/users", "GET"},
		{"/api/v1/users/batch", "POST"},
		{"/api/v1/users/search", "GET"},
	}

	for _, route := range memberRoutes {
		initialization.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// SSH web client route
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/ssh", "GET")

	// --- Admin routes ---

	adminRoutes := []struct {
		path   string
		method string
	}{
		// User deletion
		{"/api/v1/users/:id", "DELETE"},
		// Access management
		{"/api/v1/accesses", "POST"},
		{"/api/v1/accesses", "DELETE"},
		// Entity hooks management
		{"/api/v1/hooks", "GET"},
		{"/api/v1/hooks/:hook_name/enable", "POST"},
		{"/api/v1/hooks/:hook_name/disable", "POST"},
		// Email template testing
		{"/api/v1/email-templates/:id/test", "POST"},
	}

	for _, route := range adminRoutes {
		initialization.ReconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	log.Println("=== User management and access control permissions setup completed ===")
}
