package securityAdminRoutes

import (
	"log"

	"soli/formations/src/auth/interfaces"
	access "soli/formations/src/auth/access"
)

// RegisterSecurityAdminPermissions registers RBAC policies for security admin panel routes.
func RegisterSecurityAdminPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering security admin permissions ===")

	// /permissions/reference is mounted WITHOUT AuthManagement() (public endpoint),
	// so it carries NoGateway: it must be declared in the registry for the reference
	// page but must NOT get a Casbin policy.
	access.RegisterEnforced(enforcer, "Security Administration",
		access.RoutePermission{Path: "/api/v1/admin/security/policies", Method: "GET", Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly}, Description: "View all access control policies"},
		access.RoutePermission{Path: "/api/v1/admin/security/user-permissions", Method: "GET", Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly}, Description: "Look up user permissions"},
		access.RoutePermission{Path: "/api/v1/admin/security/entity-roles", Method: "GET", Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly}, Description: "View entity role matrix"},
		access.RoutePermission{Path: "/api/v1/admin/security/health-checks", Method: "GET", Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly}, Description: "Run policy health checks"},
		access.RoutePermission{Path: "/api/v1/permissions/reference", Method: "GET", NoGateway: true, Role: access.RoleMember, Access: access.AccessRule{Type: access.Public}, Description: "View permission reference page"},
	)

	log.Println("=== Security admin permissions registered ===")
}
