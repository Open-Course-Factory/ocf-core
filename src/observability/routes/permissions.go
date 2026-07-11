package routes

import (
	"log"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/interfaces"
)

// RegisterPermissions registers the Casbin policy and the declarative
// RouteRegistry entry for GET /admin/observability-metrics.
//
// The endpoint surfaces aggregated counters for Stripe sync, scenario setup,
// and hook errors — administrator only, never reachable by regular members.
func RegisterPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering observability permissions ===")

	access.RegisterEnforced(enforcer, "Observability",
		access.RoutePermission{
			Path: "/api/v1/admin/observability-metrics", Method: "GET",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Get aggregated counters for Stripe sync / scenario setup / hook errors (admin only)",
		},
	)

	log.Println("=== Observability permissions registered ===")
}
