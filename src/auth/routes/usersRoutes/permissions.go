package userController

import (
	"log"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/interfaces"
)

// RegisterUserPermissions registers RBAC permissions for user management,
// access control, entity hooks, email templates, and SSH routes.
func RegisterUserPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up user management and access control permissions ===")

	access.RegisterEnforced(enforcer, "User Management",
		access.RoutePermission{
			Path: "/api/v1/users", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "List users",
		},
		access.RoutePermission{
			Path: "/api/v1/users/batch", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Batch lookup users by IDs",
		},
		access.RoutePermission{
			Path: "/api/v1/users/search", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Search users",
		},
		access.RoutePermission{
			Path: "/api/v1/users/:id", Method: "DELETE",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Delete a user (admin only)",
		},
	)

	access.RegisterEnforced(enforcer, "Access Control",
		access.RoutePermission{
			Path: "/api/v1/accesses", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Grant access to a user (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/accesses", Method: "DELETE",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Revoke access from a user (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/hooks", Method: "GET",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "List entity hooks (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/hooks/:hook_name/enable", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Enable an entity hook (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/hooks/:hook_name/disable", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Disable an entity hook (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/email-templates/:id/test", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Send a test email template (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/ssh", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "SSH web client proxy",
		},
	)

	log.Println("=== User management and access control permissions setup completed ===")
}

// RegisterAuthPermissions registers RBAC policies for core authentication routes.
func RegisterAuthPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering authentication permissions ===")

	// Registered member-only (not fanned out over every Casdoor role that maps
	// to Member). A startup invariant guarantees every active user carries the
	// `member` role and the gateway grants if ANY of the user's roles matches,
	// so `member` alone authorizes everyone — the extra Casdoor role strings
	// never uniquely authorized a request.
	access.RegisterEnforced(enforcer, "Authentication",
		access.RoutePermission{
			Path: "/api/v1/users/:id", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get a user's profile (scoped to own ID)",
		},
		// /api/v1/users/me/* — split into per-method entries because the Layer2
		// registry Lookup does exact-match on method+path; a regex-style
		// "(GET|POST|PATCH|DELETE)" method never matches concrete requests
		// and would silently bypass the declared SelfScoped rule.
		access.RoutePermission{
			Path: "/api/v1/users/me/*", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Manage own user sub-resources",
		},
		access.RoutePermission{
			Path: "/api/v1/users/me/*", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Manage own user sub-resources",
		},
		access.RoutePermission{
			Path: "/api/v1/users/me/*", Method: "PATCH",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Manage own user sub-resources",
		},
		access.RoutePermission{
			Path: "/api/v1/users/me/*", Method: "DELETE",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Manage own user sub-resources",
		},
		access.RoutePermission{
			Path: "/api/v1/users/me/account", Method: "DELETE",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Permanently delete own account and all associated data (RGPD right to erasure)",
		},
		access.RoutePermission{
			Path: "/api/v1/auth/permissions", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get own permissions",
		},
		access.RoutePermission{
			Path: "/api/v1/auth/me", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get own authentication info",
		},
		access.RoutePermission{
			Path: "/api/v1/auth/verify-status", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Verify own authentication status",
		},
	)

	log.Println("=== Authentication permissions registered ===")
}

// RegisterFeedbackPermissions registers RBAC policies for feedback routes.
func RegisterFeedbackPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering feedback permissions ===")

	access.RegisterEnforced(enforcer, "Feedback",
		access.RoutePermission{
			Path: "/api/v1/feedback/*", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Submit feedback (scoped to authenticated user)",
		},
	)

	log.Println("=== Feedback permissions registered ===")
}
