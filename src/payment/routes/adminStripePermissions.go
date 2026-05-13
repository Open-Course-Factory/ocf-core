package paymentController

import (
	"log"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/interfaces"
)

// RegisterAdminStripePermissions registers the Casbin policy and the
// declarative RouteRegistry entry for GET /admin/stripe/pending-syncs.
//
// The endpoint surfaces pending Stripe sync queue rows — administrator only,
// never reachable by regular members. Mirrors the observability admin route.
func RegisterAdminStripePermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering admin stripe pending-syncs permissions ===")

	// Layer 1 (RBAC): only administrator can read pending Stripe syncs.
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/admin/stripe/pending-syncs", "GET")

	// Layer 2 (Business Logic): AdminOnly access rule.
	access.RouteRegistry.Register("Admin Stripe",
		access.RoutePermission{
			Path: "/api/v1/admin/stripe/pending-syncs", Method: "GET",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "List pending Stripe sync queue rows (admin only)",
		},
	)

	log.Println("=== Admin stripe pending-syncs permissions registered ===")
}
