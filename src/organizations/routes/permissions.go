package routes

import (
	"log"

	"soli/formations/src/auth/interfaces"
	casbinUtils "soli/formations/src/auth/casbin"
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
		casbinUtils.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Admin-only routes — handler also has isAdmin() check
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/organizations/:id/backends", "PUT")

	// --- Route Registry: declarative permission metadata ---

	casbinUtils.RouteRegistry.Register("Organizations",
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/members", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization members",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/groups", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization groups",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/import", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Import members into an organization",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/groups/:groupId/regenerate-passwords", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Regenerate passwords for a group in the organization",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/convert-to-team", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "owner"},
			Description: "Convert organization to a team",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/backends", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization backends",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/backends", Method: "PUT",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Update organization backends (admin only)",
		},
	)

	log.Println("=== Organization custom route permissions registered ===")
}
