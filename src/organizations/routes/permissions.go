package routes

import (
	"log"

	"soli/formations/src/auth/interfaces"
	"soli/formations/src/initialization"
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
		initialization.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Admin-only routes — handler also has isAdmin() check
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/organizations/:id/backends", "PUT")

	log.Println("=== Organization custom route permissions registered ===")
}
