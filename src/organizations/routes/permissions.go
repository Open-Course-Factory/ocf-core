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
			Path: "/api/v1/organizations/:id/members/offboard", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Offboard members: deactivate, block sign-in, schedule erasure after the retention period",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/members/:userId/reinstate", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Reinstate an offboarded member",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/members/:userId/erase", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "owner"},
			Description: "Erase an offboarded member now, ahead of the scheduled date (owner only)",
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
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/ai-config", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "Read whether the organization's AI-assisted scenario authoring is configured (never the key)",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/ai-config", Method: "PUT",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Configure the organization's own AI assistant endpoint, model and key (manager and owner only)",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/ai-config/generate", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Generate a scenario answer with the organization's configured AI assistant (manager and owner only)",
		},
	)

	log.Println("=== Organization custom route permissions registered ===")
}
