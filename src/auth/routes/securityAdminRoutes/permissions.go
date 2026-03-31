package securityAdminRoutes

import (
	"log"

	"soli/formations/src/auth/interfaces"
	casbinUtils "soli/formations/src/auth/casbin"
)

// RegisterSecurityAdminPermissions registers Casbin policies for security admin panel routes.
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
		casbinUtils.ReconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	// Permission reference — available to all authenticated users
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/permissions/reference", "GET")

	casbinUtils.RouteRegistry.Register("Security Administration",
		casbinUtils.RoutePermission{Path: "/api/v1/admin/security/policies", Method: "GET", Role: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly}, Description: "View all Casbin policies"},
		casbinUtils.RoutePermission{Path: "/api/v1/admin/security/user-permissions", Method: "GET", Role: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly}, Description: "Look up user permissions"},
		casbinUtils.RoutePermission{Path: "/api/v1/admin/security/entity-roles", Method: "GET", Role: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly}, Description: "View entity role matrix"},
		casbinUtils.RoutePermission{Path: "/api/v1/admin/security/health-checks", Method: "GET", Role: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly}, Description: "Run policy health checks"},
		casbinUtils.RoutePermission{Path: "/api/v1/permissions/reference", Method: "GET", Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public}, Description: "View permission reference page"},
	)

	log.Println("=== Security admin permissions registered ===")
}
