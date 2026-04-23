package paymentController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	access "soli/formations/src/auth/access"
)

// RegisterPaymentPermissions registers all Casbin policies for payment routes.
// This replaces the centralized SetupPaymentPermissions in initialization/permissions.go.
func RegisterPaymentPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering payment module permissions ===")

	registerUserSubscriptionPermissions(enforcer)
	registerOrganizationSubscriptionPermissions(enforcer)
	registerInvoicePermissions(enforcer)
	registerPaymentMethodPermissions(enforcer)
	registerBillingAddressPermissions(enforcer)
	registerUsageMetricsPermissions(enforcer)
	registerSubscriptionPlanPermissions(enforcer)
	registerHooksPermissions(enforcer)
	registerBulkLicensePermissions(enforcer)

	// --- Route registry declarations (declarative permission metadata) ---

	access.RouteRegistry.Register("Subscriptions",
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/current", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get current user subscription",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/all", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List all user subscriptions",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/usage", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get subscription usage summary",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/checkout", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Create Stripe checkout session",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/portal", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Open Stripe billing portal",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/:id/cancel", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Cancel a subscription",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/:id/reactivate", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Reactivate a cancelled subscription",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/upgrade", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Upgrade subscription plan",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/usage/check", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Check usage limit availability",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/sync-usage-limits", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Sync usage limits from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/purchase-bulk", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Purchase bulk license batch",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/analytics", Method: "GET",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "View subscription analytics",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/admin-assign", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Admin-assign subscription to user",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/sync-existing", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Sync all existing Stripe subscriptions",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/users/:user_id/sync", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Sync Stripe subscription for specific user",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/sync-missing-metadata", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Sync missing Stripe metadata",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/link/:subscription_id", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Link Stripe subscription to user",
		},
	)

	access.RouteRegistry.Register("Organization Subscriptions",
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/subscribe", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Subscribe organization to a plan (manager+)",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/subscription", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "Get organization subscription",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/subscription", Method: "DELETE",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Cancel organization subscription (manager+)",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/features", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization features",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/usage-limits", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "Get organization usage limits",
		},
		access.RoutePermission{
			Path: "/api/v1/users/me/features", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List features available to current user",
		},
		access.RoutePermission{
			Path: "/api/v1/admin/organizations/subscriptions", Method: "GET",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "List all organization subscriptions (admin)",
		},
	)

	access.RouteRegistry.Register("Billing",
		access.RoutePermission{
			Path: "/api/v1/invoices/user", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List user invoices",
		},
		access.RoutePermission{
			Path: "/api/v1/invoices/sync", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Sync invoices from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/invoices/:id/download", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Download invoice PDF",
		},
		access.RoutePermission{
			Path: "/api/v1/invoices/admin/cleanup", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Clean up orphaned invoices",
		},
		access.RoutePermission{
			Path: "/api/v1/payment-methods/user", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List user payment methods",
		},
		access.RoutePermission{
			Path: "/api/v1/payment-methods/sync", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Sync payment methods from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/payment-methods/:id/set-default", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Set default payment method",
		},
		access.RoutePermission{
			Path: "/api/v1/billing-addresses/user", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List user billing addresses",
		},
		access.RoutePermission{
			Path: "/api/v1/billing-addresses/:id/set-default", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Set default billing address",
		},
	)

	access.RouteRegistry.Register("Usage & Plans",
		access.RoutePermission{
			Path: "/api/v1/usage-metrics/user", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get user usage metrics",
		},
		access.RoutePermission{
			Path: "/api/v1/usage-metrics/increment", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Increment usage metric counter (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/usage-metrics/reset", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Reset usage metric counter (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-plans/:id/sync-stripe", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Sync single plan from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-plans/sync-stripe", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Sync all plans from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-plans/import-stripe", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Import plans from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/hooks/stripe/toggle", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Toggle Stripe webhook processing",
		},
	)

	access.RouteRegistry.Register("Bulk Licenses",
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/create-checkout-session", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Create batch checkout session",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "List owned subscription batches",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Get subscription batch details",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/licenses", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "List licenses in a batch",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/assign", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Assign license from batch to user",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/licenses/:license_id/revoke", Method: "DELETE",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Revoke a batch license",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/quantity", Method: "PATCH",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Update batch license quantity",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/permanent", Method: "DELETE",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Permanently delete a batch",
		},
	)

	log.Println("=== Payment module permissions registered ===")
}

// registerUserSubscriptionPermissions registers policies for /api/v1/user-subscriptions/*
func registerUserSubscriptionPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering user subscription permissions...")

	// Read-only routes (no email verification required)
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/current", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/all", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/usage", "GET")

	// Payment actions (require verified email)
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/checkout", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/portal", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/:id/cancel", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/:id/reactivate", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/upgrade", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/usage/check", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/sync-usage-limits", "POST")

	// Admin routes (Layer 1 restricts to administrator, Layer 2 enforces AdminOnly)
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/analytics", "GET")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/admin-assign", "POST")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/sync-existing", "POST")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/users/:user_id/sync", "POST")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/sync-missing-metadata", "POST")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/link/:subscription_id", "POST")
}

// registerOrganizationSubscriptionPermissions registers policies for organization subscription routes
func registerOrganizationSubscriptionPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering organization subscription permissions...")

	// Organization subscription management (member-scoped, Layer 2 enforces OrgRole with admin bypass)
	access.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/subscribe", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/subscription", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/subscription", "DELETE")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/features", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/usage-limits", "GET")

	// User feature access
	access.ReconcilePolicy(enforcer, "member", "/api/v1/users/me/features", "GET")

	// Admin bulk routes
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/admin/organizations/subscriptions", "GET")
}

// registerInvoicePermissions registers policies for /api/v1/invoices/*
func registerInvoicePermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering invoice permissions...")

	// Member routes (self-scoped)
	access.ReconcilePolicy(enforcer, "member", "/api/v1/invoices/user", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/invoices/sync", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/invoices/:id/download", "GET")

	// Admin routes
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/invoices/admin/cleanup", "POST")
}

// registerPaymentMethodPermissions registers policies for /api/v1/payment-methods/*
func registerPaymentMethodPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering payment method permissions...")

	access.ReconcilePolicy(enforcer, "member", "/api/v1/payment-methods/user", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/payment-methods/sync", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/payment-methods/:id/set-default", "POST")
}

// registerBillingAddressPermissions registers policies for /api/v1/billing-addresses/*
func registerBillingAddressPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering billing address permissions...")

	access.ReconcilePolicy(enforcer, "member", "/api/v1/billing-addresses/user", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/billing-addresses/:id/set-default", "POST")
}

// registerUsageMetricsPermissions registers policies for /api/v1/usage-metrics/*
func registerUsageMetricsPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering usage metrics permissions...")

	access.ReconcilePolicy(enforcer, "member", "/api/v1/usage-metrics/user", "GET")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/usage-metrics/increment", "POST")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/usage-metrics/reset", "POST")
}

// registerSubscriptionPlanPermissions registers policies for /api/v1/subscription-plans/*
func registerSubscriptionPlanPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering subscription plan permissions...")

	// Note: /api/v1/subscription-plans/pricing-preview is public (no auth), no Casbin policy needed

	// Admin routes (Stripe sync)
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/:id/sync-stripe", "POST")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/sync-stripe", "POST")
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/import-stripe", "POST")
}

// registerHooksPermissions registers policies for /api/v1/hooks/*
func registerHooksPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering hooks permissions...")

	// Admin only
	access.ReconcilePolicy(enforcer, "administrator", "/api/v1/hooks/stripe/toggle", "POST")
}

// registerBulkLicensePermissions registers policies for bulk license and subscription batch routes
func registerBulkLicensePermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering bulk license permissions...")

	// Bulk purchase (member-scoped, role checks in controller)
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/purchase-bulk", "POST")

	// Subscription batch management (member-scoped, ownership checks in controller)
	access.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/create-checkout-session", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/licenses", "GET")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/assign", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/licenses/:license_id/revoke", "DELETE")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/quantity", "PATCH")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/permanent", "DELETE")
}
