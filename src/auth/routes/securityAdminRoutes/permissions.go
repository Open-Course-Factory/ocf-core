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

	log.Println("=== Security admin permissions registered ===")
}
