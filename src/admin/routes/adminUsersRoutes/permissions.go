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
	)

	log.Println("=== Admin users permissions registered ===")
}
