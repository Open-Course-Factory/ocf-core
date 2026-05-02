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

	// Member routes — org membership is verified in handlers
	memberRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/organizations/me/memberships", "GET"},
		{"/api/v1/organizations/:id/members", "GET"},
		{"/api/v1/organizations/:id/groups", "GET"},
		{"/api/v1/organizations/:id/convert-to-team", "POST"},
		{"/api/v1/organizations/:id/backends", "GET"},
		// Import and password regen are member at Casbin level;
		// handlers enforce org owner/manager/admin role (Layer 2)
		{"/api/v1/organizations/:id/import", "POST"},
		{"/api/v1/organizations/:id/groups/:groupId/regenerate-passwords", "POST"},
	}

	for _, route := range memberRoutes {
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Admin-only routes (Layer 1 restricts to administrator, Layer 2 enforces AdminOnly)
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/organizations/:id/backends", "PUT")

	// --- Route Registry: declarative permission metadata ---

	access.RouteRegistry.Register("Organizations",
		access.RoutePermission{
			Path: "/api/v1/organizations/me/memberships", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List the authenticated user's organization memberships and roles",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/members", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization members",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/groups", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization groups",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/import", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Import members into an organization",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/groups/:groupId/regenerate-passwords", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Regenerate passwords for a group in the organization",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/convert-to-team", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "owner"},
			Description: "Convert organization to a team",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/backends", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization backends",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/backends", Method: "PUT",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Update organization backends (admin only)",
		},
	)

	log.Println("=== Organization custom route permissions registered ===")
}
