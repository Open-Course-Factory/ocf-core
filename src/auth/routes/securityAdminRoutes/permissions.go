package securityAdminRoutes

import (
	"log"

	"soli/formations/src/auth/interfaces"
	access "soli/formations/src/auth/access"
)

// RegisterSecurityAdminPermissions registers RBAC policies for security admin panel routes.
func RegisterSecurityAdminPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering security admin permissions ===")

	adminRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/admin/security/policies", "GET"},
		{"/api/v1/admin/security/user-permissions", "GET"},
		{"/api/v1/admin/security/entity-roles", "GET"},
		{"/api/v1/admin/security/health-checks", "GET"},
	}

	for _, route := range adminRoutes {
		access.ReconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	// Note: /permissions/reference is registered without AuthManagement() middleware
	// (public endpoint), so no Casbin policy is needed — any authenticated or
	// unauthenticated user can access it. Only the RouteRegistry declaration below
	// is needed for the reference page itself.

	access.RouteRegistry.Register("Security Administration",
		access.RoutePermission{Path: "/api/v1/admin/security/policies", Method: "GET", Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly}, Description: "View all access control policies"},
		access.RoutePermission{Path: "/api/v1/admin/security/user-permissions", Method: "GET", Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly}, Description: "Look up user permissions"},
		access.RoutePermission{Path: "/api/v1/admin/security/entity-roles", Method: "GET", Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly}, Description: "View entity role matrix"},
		access.RoutePermission{Path: "/api/v1/admin/security/health-checks", Method: "GET", Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly}, Description: "Run policy health checks"},
		access.RoutePermission{Path: "/api/v1/permissions/reference", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.Public}, Description: "View permission reference page"},
	)

	log.Println("=== Security admin permissions registered ===")
}
