package adminUsersRoutes

import (
	"log"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/interfaces"
)

// RegisterPermissions registers the Casbin policy and the declarative
// RouteRegistry entry for GET /admin/users-with-memberships.
//
// The endpoint is administrator-only — it surfaces the entire user
// directory along with org and group memberships, so it must never be
// reachable by regular members.
func RegisterPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering admin users permissions ===")

	access.RegisterEnforced(enforcer, "Admin Users",
		access.RoutePermission{
			Path: "/api/v1/admin/users-with-memberships", Method: "GET",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "List all users with their organization and group memberships (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/admin/users/:id/send-password-reset-email", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Send a password reset email to a user on an admin's behalf (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/admin/users/:id/admin-role", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Grant the administrator role to a user (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/admin/users/:id/admin-role", Method: "DELETE",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Revoke the administrator role from a user (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/admin/users/:id/forbidden", Method: "PUT",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Block or unblock a user's account sign-in (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/admin/users", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Create a user account directly, bypassing self-signup (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/admin/users/:id/verify-email", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Force-verify a user's email with no token (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/admin/users/:id/password", Method: "PUT",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Set a user's password directly, with no email sent (admin only)",
		},
	)

	log.Println("=== Admin users permissions registered ===")
}
