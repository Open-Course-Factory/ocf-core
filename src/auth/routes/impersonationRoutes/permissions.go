package impersonationRoutes

import (
	"log"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/interfaces"
)

// RegisterImpersonationPermissions registers the Casbin policies and the
// declarative RouteRegistry entries for the three platform-admin
// impersonation endpoints.
//
// All three routes are administrator-only — granting a non-admin the
// ability to impersonate anyone would defeat the entire access-control
// model.
func RegisterImpersonationPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering impersonation permissions ===")

	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/admin/impersonate/start", "POST")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/admin/impersonate/stop", "POST")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/admin/impersonate/active", "GET")

	access.RouteRegistry.Register("Impersonation",
		access.RoutePermission{
			Path: "/api/v1/admin/impersonate/start", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Start impersonating a user (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/admin/impersonate/stop", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Stop the current impersonation session",
		},
		access.RoutePermission{
			Path: "/api/v1/admin/impersonate/active", Method: "GET",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Get the admin's currently active impersonation session",
		},
	)

	log.Println("=== Impersonation permissions registered ===")
}
