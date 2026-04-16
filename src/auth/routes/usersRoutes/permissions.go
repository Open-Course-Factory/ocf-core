package userController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
	access "soli/formations/src/auth/access"
)

// RegisterUserPermissions registers RBAC permissions for user management,
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
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// SSH web client route
	access.ReconcilePolicy(enforcer, "member", "/api/v1/ssh", "GET")

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
		access.ReconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	// --- Route Registry: declarative permission metadata ---

	access.RouteRegistry.Register("User Management",
		access.RoutePermission{
			Path: "/api/v1/users", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "List users",
		},
		access.RoutePermission{
			Path: "/api/v1/users/batch", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Batch lookup users by IDs",
		},
		access.RoutePermission{
			Path: "/api/v1/users/search", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Search users",
		},
		access.RoutePermission{
			Path: "/api/v1/users/:id", Method: "DELETE",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Delete a user (admin only)",
		},
	)

	access.RouteRegistry.Register("Access Control",
		access.RoutePermission{
			Path: "/api/v1/accesses", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Grant access to a user (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/accesses", Method: "DELETE",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Revoke access from a user (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/hooks", Method: "GET",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "List entity hooks (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/hooks/:hook_name/enable", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Enable an entity hook (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/hooks/:hook_name/disable", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Disable an entity hook (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/email-templates/:id/test", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Send a test email template (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/ssh", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "SSH web client proxy",
		},
	)

	log.Println("=== User management and access control permissions setup completed ===")
}

// RegisterAuthPermissions registers RBAC policies for core authentication routes.
// These are registered per-Casdoor-role (not just "member") because the auth
// middleware resolves the user's actual Casdoor role before checking permissions.
func RegisterAuthPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering authentication permissions ===")

	casdoorRoles := authModels.GetCasdoorRolesForOCFRole(authModels.Member)

	for _, role := range casdoorRoles {
		access.ReconcilePolicy(enforcer, role, "/api/v1/users/:id", "GET")
		access.ReconcilePolicy(enforcer, role, "/api/v1/users/me/*", "(GET|POST|PATCH|DELETE)")
		access.ReconcilePolicy(enforcer, role, "/api/v1/auth/permissions", "GET")
		access.ReconcilePolicy(enforcer, role, "/api/v1/auth/me", "GET")
		access.ReconcilePolicy(enforcer, role, "/api/v1/auth/verify-status", "GET")
	}

	// --- Route Registry: declarative permission metadata ---

	access.RouteRegistry.Register("Authentication",
		access.RoutePermission{
			Path: "/api/v1/users/:id", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get a user's profile (scoped to own ID)",
		},
		access.RoutePermission{
			Path: "/api/v1/users/me/*", Method: "(GET|POST|PATCH|DELETE)",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Manage own user sub-resources",
		},
		access.RoutePermission{
			Path: "/api/v1/users/me/account", Method: "DELETE",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Permanently delete own account and all associated data (RGPD right to erasure)",
		},
		access.RoutePermission{
			Path: "/api/v1/auth/permissions", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get own permissions",
		},
		access.RoutePermission{
			Path: "/api/v1/auth/me", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get own authentication info",
		},
		access.RoutePermission{
			Path: "/api/v1/auth/verify-status", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Verify own authentication status",
		},
	)

	log.Println("=== Authentication permissions registered ===")
}

// RegisterFeedbackPermissions registers RBAC policies for feedback routes.
func RegisterFeedbackPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering feedback permissions ===")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/feedback/*", "POST")

	// --- Route Registry: declarative permission metadata ---

	access.RouteRegistry.Register("Feedback",
		access.RoutePermission{
			Path: "/api/v1/feedback/*", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Submit feedback (scoped to authenticated user)",
		},
	)

	log.Println("=== Feedback permissions registered ===")
}
