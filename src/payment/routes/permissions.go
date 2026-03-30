package paymentController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	casbinUtils "soli/formations/src/auth/casbin"
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

	casbinUtils.RouteRegistry.Register("Subscriptions",
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/current", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Get current user subscription",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/all", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "List all user subscriptions",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/usage", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Get subscription usage summary",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/checkout", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Create Stripe checkout session",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/portal", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Open Stripe billing portal",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/:id/cancel", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Cancel a subscription",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/:id/reactivate", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Reactivate a cancelled subscription",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/upgrade", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Upgrade subscription plan",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/usage/check", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Check usage limit availability",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/sync-usage-limits", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Sync usage limits from Stripe",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/purchase-bulk", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Purchase bulk license batch",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/analytics", Method: "GET",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "View subscription analytics",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/admin-assign", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Admin-assign subscription to user",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/sync-existing", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Sync all existing Stripe subscriptions",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/users/:user_id/sync", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Sync Stripe subscription for specific user",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/sync-missing-metadata", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Sync missing Stripe metadata",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/user-subscriptions/link/:subscription_id", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Link Stripe subscription to user",
		},
	)

	casbinUtils.RouteRegistry.Register("Organization Subscriptions",
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/subscribe", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "member"},
			Description: "Subscribe organization to a plan",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/subscription", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "member"},
			Description: "Get organization subscription",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/subscription", Method: "DELETE",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "member"},
			Description: "Cancel organization subscription",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/features", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization features",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/usage-limits", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "member"},
			Description: "Get organization usage limits",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/users/me/features", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "List features available to current user",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/admin/organizations/subscriptions", Method: "GET",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "List all organization subscriptions (admin)",
		},
	)

	casbinUtils.RouteRegistry.Register("Billing",
		casbinUtils.RoutePermission{
			Path: "/api/v1/invoices/user", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "List user invoices",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/invoices/sync", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Sync invoices from Stripe",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/invoices/:id/download", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Download invoice PDF",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/invoices/admin/cleanup", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Clean up orphaned invoices",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/payment-methods/user", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "List user payment methods",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/payment-methods/sync", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Sync payment methods from Stripe",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/payment-methods/:id/set-default", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Set default payment method",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/billing-addresses/user", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "List user billing addresses",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/billing-addresses/:id/set-default", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Set default billing address",
		},
	)

	casbinUtils.RouteRegistry.Register("Usage & Plans",
		casbinUtils.RoutePermission{
			Path: "/api/v1/usage-metrics/user", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Get user usage metrics",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/usage-metrics/increment", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Increment usage metric counter",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/usage-metrics/reset", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Reset usage metric counter",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/subscription-plans/:id/sync-stripe", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Sync single plan from Stripe",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/subscription-plans/sync-stripe", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Sync all plans from Stripe",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/subscription-plans/import-stripe", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Import plans from Stripe",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/hooks/stripe/toggle", Method: "POST",
			CasbinRole: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Toggle Stripe webhook processing",
		},
	)

	casbinUtils.RouteRegistry.Register("Bulk Licenses",
		casbinUtils.RoutePermission{
			Path: "/api/v1/subscription-batches/create-checkout-session", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserID"},
			Description: "Create batch checkout session",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/subscription-batches", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserID"},
			Description: "List owned subscription batches",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/subscription-batches/:id", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserID"},
			Description: "Get subscription batch details",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/licenses", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserID"},
			Description: "List licenses in a batch",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/assign", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserID"},
			Description: "Assign license from batch to user",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/licenses/:license_id/revoke", Method: "DELETE",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserID"},
			Description: "Revoke a batch license",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/quantity", Method: "PATCH",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserID"},
			Description: "Update batch license quantity",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/permanent", Method: "DELETE",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserID"},
			Description: "Permanently delete a batch",
		},
	)

	log.Println("=== Payment module permissions registered ===")
}

// registerUserSubscriptionPermissions registers policies for /api/v1/user-subscriptions/*
func registerUserSubscriptionPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering user subscription permissions...")

	// Read-only routes (no email verification required)
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/current", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/all", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/usage", "GET")

	// Payment actions (require verified email)
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/checkout", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/portal", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/:id/cancel", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/:id/reactivate", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/upgrade", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/usage/check", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/sync-usage-limits", "POST")

	// Admin routes (handlers have isAdmin() checks)
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/analytics", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/admin-assign", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/sync-existing", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/users/:user_id/sync", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/sync-missing-metadata", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/link/:subscription_id", "POST")
}

// registerOrganizationSubscriptionPermissions registers policies for organization subscription routes
func registerOrganizationSubscriptionPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering organization subscription permissions...")

	// Organization subscription management (member-scoped, fine-grained checks in controller)
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/subscribe", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/subscription", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/subscription", "DELETE")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/features", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/usage-limits", "GET")

	// User feature access
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/users/me/features", "GET")

	// Admin bulk routes
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/admin/organizations/subscriptions", "GET")
}

// registerInvoicePermissions registers policies for /api/v1/invoices/*
func registerInvoicePermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering invoice permissions...")

	// Member routes (self-scoped)
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/invoices/user", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/invoices/sync", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/invoices/:id/download", "GET")

	// Admin routes
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/invoices/admin/cleanup", "POST")
}

// registerPaymentMethodPermissions registers policies for /api/v1/payment-methods/*
func registerPaymentMethodPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering payment method permissions...")

	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/payment-methods/user", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/payment-methods/sync", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/payment-methods/:id/set-default", "POST")
}

// registerBillingAddressPermissions registers policies for /api/v1/billing-addresses/*
func registerBillingAddressPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering billing address permissions...")

	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/billing-addresses/user", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/billing-addresses/:id/set-default", "POST")
}

// registerUsageMetricsPermissions registers policies for /api/v1/usage-metrics/*
func registerUsageMetricsPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering usage metrics permissions...")

	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/usage-metrics/user", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/usage-metrics/increment", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/usage-metrics/reset", "POST")
}

// registerSubscriptionPlanPermissions registers policies for /api/v1/subscription-plans/*
func registerSubscriptionPlanPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering subscription plan permissions...")

	// Note: /api/v1/subscription-plans/pricing-preview is public (no auth), no Casbin policy needed

	// Admin routes (Stripe sync)
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/:id/sync-stripe", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/sync-stripe", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/import-stripe", "POST")
}

// registerHooksPermissions registers policies for /api/v1/hooks/*
func registerHooksPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering hooks permissions...")

	// Admin only
	casbinUtils.ReconcilePolicy(enforcer, "administrator", "/api/v1/hooks/stripe/toggle", "POST")
}

// registerBulkLicensePermissions registers policies for bulk license and subscription batch routes
func registerBulkLicensePermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering bulk license permissions...")

	// Bulk purchase (member-scoped, role checks in controller)
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/purchase-bulk", "POST")

	// Subscription batch management (member-scoped, ownership checks in controller)
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/create-checkout-session", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/licenses", "GET")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/assign", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/licenses/:license_id/revoke", "DELETE")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/quantity", "PATCH")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/permanent", "DELETE")
}
