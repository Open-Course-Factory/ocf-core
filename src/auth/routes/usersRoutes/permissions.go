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

	// --- Route Registry: declarative permission metadata ---

	casbinUtils.RouteRegistry.Register("User Management",
		casbinUtils.RoutePermission{
			Path: "/api/v1/users", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "List users",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/users/batch", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Batch lookup users by IDs",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/users/search", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Search users",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/users/:id", Method: "DELETE",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Delete a user (admin only)",
		},
	)

	casbinUtils.RouteRegistry.Register("Access Control",
		casbinUtils.RoutePermission{
			Path: "/api/v1/accesses", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Grant access to a user (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/accesses", Method: "DELETE",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Revoke access from a user (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/hooks", Method: "GET",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "List entity hooks (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/hooks/:hook_name/enable", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Enable an entity hook (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/hooks/:hook_name/disable", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Disable an entity hook (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/email-templates/:id/test", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Send a test email template (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/ssh", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "SSH web client proxy",
		},
	)

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

	// --- Route Registry: declarative permission metadata ---

	casbinUtils.RouteRegistry.Register("Authentication",
		casbinUtils.RoutePermission{
			Path: "/api/v1/users/:id", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Get a user's profile (scoped to own ID)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/users/me/*", Method: "(GET|POST|PATCH|DELETE)",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Manage own user sub-resources",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/auth/permissions", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Get own permissions",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/auth/me", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Get own authentication info",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/auth/verify-status", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Verify own authentication status",
		},
	)

	log.Println("=== Authentication permissions registered ===")
}

// RegisterFeedbackPermissions registers Casbin policies for feedback routes.
func RegisterFeedbackPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering feedback permissions ===")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/feedback/*", "POST")

	// --- Route Registry: declarative permission metadata ---

	casbinUtils.RouteRegistry.Register("Feedback",
		casbinUtils.RoutePermission{
			Path: "/api/v1/feedback/*", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Submit feedback (scoped to authenticated user)",
		},
	)

	log.Println("=== Feedback permissions registered ===")
}
