package routes

import (
	"log"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/interfaces"
)

// RegisterGroupPermissions registers Casbin policies for custom group routes.
// CRUD policies for the ClassGroup entity are registered automatically via
// entity registration; only handcrafted custom routes live here.
func RegisterGroupPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering group custom route permissions ===")

	// Member routes — handler-level scoping (controller verifies userId).
	memberRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/groups/me/memberships", "GET"},
	}

	for _, route := range memberRoutes {
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// --- Route Registry: declarative permission metadata ---

	access.RouteRegistry.Register("Groups",
		access.RoutePermission{
			Path: "/api/v1/groups/me/memberships", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List the authenticated user's group memberships and roles",
		},
	)

	log.Println("=== Group custom route permissions registered ===")
}
