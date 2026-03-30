package userController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
	casbinUtils "soli/formations/src/auth/casbin"
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
		casbinUtils.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// SSH web client route
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/ssh", "GET")

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
		casbinUtils.ReconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	log.Println("=== User management and access control permissions setup completed ===")
}

// RegisterAuthPermissions registers Casbin policies for core authentication routes.
// These are registered per-Casdoor-role (not just "member") because the auth
// middleware resolves the user's actual Casdoor role before checking permissions.
func RegisterAuthPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering authentication permissions ===")

	casdoorRoles := authModels.GetCasdoorRolesForOCFRole(authModels.Member)

	for _, role := range casdoorRoles {
		casbinUtils.ReconcilePolicy(enforcer, role, "/api/v1/users/:id", "GET")
		casbinUtils.ReconcilePolicy(enforcer, role, "/api/v1/users/me/*", "(GET|POST|PATCH|DELETE)")
		casbinUtils.ReconcilePolicy(enforcer, role, "/api/v1/auth/permissions", "GET")
		casbinUtils.ReconcilePolicy(enforcer, role, "/api/v1/auth/me", "GET")
		casbinUtils.ReconcilePolicy(enforcer, role, "/api/v1/auth/verify-status", "GET")
	}

	log.Println("=== Authentication permissions registered ===")
}

// RegisterFeedbackPermissions registers Casbin policies for feedback routes.
func RegisterFeedbackPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering feedback permissions ===")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/feedback/*", "POST")
	log.Println("=== Feedback permissions registered ===")
}
