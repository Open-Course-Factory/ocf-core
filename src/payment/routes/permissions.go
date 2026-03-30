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
