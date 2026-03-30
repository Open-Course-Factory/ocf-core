package paymentController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	"soli/formations/src/initialization"
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

	log.Println("=== Payment module permissions registered ===")
}

// registerUserSubscriptionPermissions registers policies for /api/v1/user-subscriptions/*
func registerUserSubscriptionPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering user subscription permissions...")

	// Read-only routes (no email verification required)
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/current", "GET")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/all", "GET")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/usage", "GET")

	// Payment actions (require verified email)
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/checkout", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/portal", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/:id/cancel", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/:id/reactivate", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/upgrade", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/usage/check", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/sync-usage-limits", "POST")

	// Admin routes (handlers have isAdmin() checks)
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/analytics", "GET")
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/admin-assign", "POST")
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/sync-existing", "POST")
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/users/:user_id/sync", "POST")
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/sync-missing-metadata", "POST")
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/user-subscriptions/link/:subscription_id", "POST")
}

// registerOrganizationSubscriptionPermissions registers policies for organization subscription routes
func registerOrganizationSubscriptionPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering organization subscription permissions...")

	// Organization subscription management (member-scoped, fine-grained checks in controller)
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/subscribe", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/subscription", "GET")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/subscription", "DELETE")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/features", "GET")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/usage-limits", "GET")

	// User feature access
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/users/me/features", "GET")

	// Admin bulk routes
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/admin/organizations/subscriptions", "GET")
}

// registerInvoicePermissions registers policies for /api/v1/invoices/*
func registerInvoicePermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering invoice permissions...")

	// Member routes (self-scoped)
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/invoices/user", "GET")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/invoices/sync", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/invoices/:id/download", "GET")

	// Admin routes
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/invoices/admin/cleanup", "POST")
}

// registerPaymentMethodPermissions registers policies for /api/v1/payment-methods/*
func registerPaymentMethodPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering payment method permissions...")

	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/payment-methods/user", "GET")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/payment-methods/sync", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/payment-methods/:id/set-default", "POST")
}

// registerBillingAddressPermissions registers policies for /api/v1/billing-addresses/*
func registerBillingAddressPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering billing address permissions...")

	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/billing-addresses/user", "GET")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/billing-addresses/:id/set-default", "POST")
}

// registerUsageMetricsPermissions registers policies for /api/v1/usage-metrics/*
func registerUsageMetricsPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering usage metrics permissions...")

	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/usage-metrics/user", "GET")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/usage-metrics/increment", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/usage-metrics/reset", "POST")
}

// registerSubscriptionPlanPermissions registers policies for /api/v1/subscription-plans/*
func registerSubscriptionPlanPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering subscription plan permissions...")

	// Note: /api/v1/subscription-plans/pricing-preview is public (no auth), no Casbin policy needed

	// Admin routes (Stripe sync)
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/:id/sync-stripe", "POST")
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/sync-stripe", "POST")
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/subscription-plans/import-stripe", "POST")
}

// registerHooksPermissions registers policies for /api/v1/hooks/*
func registerHooksPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering hooks permissions...")

	// Admin only
	initialization.ReconcilePolicy(enforcer, "administrator", "/api/v1/hooks/stripe/toggle", "POST")
}

// registerBulkLicensePermissions registers policies for bulk license and subscription batch routes
func registerBulkLicensePermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("Registering bulk license permissions...")

	// Bulk purchase (member-scoped, role checks in controller)
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/user-subscriptions/purchase-bulk", "POST")

	// Subscription batch management (member-scoped, ownership checks in controller)
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/create-checkout-session", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches", "GET")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id", "GET")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/licenses", "GET")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/assign", "POST")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/licenses/:license_id/revoke", "DELETE")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/quantity", "PATCH")
	initialization.ReconcilePolicy(enforcer, "member", "/api/v1/subscription-batches/:id/permanent", "DELETE")
}
