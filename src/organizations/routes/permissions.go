package routes

import (
	"log"

	"soli/formations/src/auth/interfaces"
	access "soli/formations/src/auth/access"
)

// RegisterOrganizationPermissions registers Casbin policies for custom organization routes.
// All member routes have handler-level org membership checks (Layer 2).
func RegisterOrganizationPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering organization custom route permissions ===")

	access.RegisterEnforced(enforcer, "Organizations",
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/members", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization members",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/groups", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization groups",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/import", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Import members into an organization",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/groups/:groupId/regenerate-passwords", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Regenerate passwords for a group in the organization",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/convert-to-team", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "owner"},
			Description: "Convert organization to a team",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/backends", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization backends",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/backends", Method: "PUT",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Update organization backends (admin only)",
		},
	)

	log.Println("=== Organization custom route permissions registered ===")
}
