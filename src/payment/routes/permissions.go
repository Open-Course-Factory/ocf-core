package paymentController

import (
	"log"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/interfaces"
)

// RegisterPaymentPermissions registers all Casbin policies for payment routes.
// This replaces the centralized SetupPaymentPermissions in initialization/permissions.go.
func RegisterPaymentPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering payment module permissions ===")

	access.RegisterEnforced(enforcer, "Subscriptions",
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/current", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get current user subscription",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/all", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List all user subscriptions",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/usage", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get subscription usage summary",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/checkout", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Create Stripe checkout session",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/portal", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Open Stripe billing portal",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/:id/cancel", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Cancel a subscription",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/:id/reactivate", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Reactivate a cancelled subscription",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/upgrade", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Upgrade subscription plan",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/usage/check", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Check usage limit availability",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/sync-usage-limits", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Sync usage limits from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/purchase-bulk", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Purchase bulk license batch",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/analytics", Method: "GET",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "View subscription analytics",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/admin-assign", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Admin-assign subscription to user",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/sync-existing", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Sync all existing Stripe subscriptions",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/users/:user_id/sync", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Sync Stripe subscription for specific user",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/sync-missing-metadata", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Sync missing Stripe metadata",
		},
		access.RoutePermission{
			Path: "/api/v1/user-subscriptions/link/:subscription_id", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Link Stripe subscription to user",
		},
	)

	access.RegisterEnforced(enforcer, "Organization Subscriptions",
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/subscribe", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Subscribe organization to a plan (manager+)",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/subscription", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "Get organization subscription",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/subscription", Method: "DELETE",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Cancel organization subscription (manager+)",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/features", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "List organization features",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/usage-limits", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"},
			Description: "Get organization usage limits",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/invoices", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "List organization invoices (manager+)",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/role-plans", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "List organization role→plan mappings (manager+)",
		},
		access.RoutePermission{
			Path: "/api/v1/users/me/features", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List features available to current user",
		},
		access.RoutePermission{
			Path: "/api/v1/admin/organizations/subscriptions", Method: "GET",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "List all organization subscriptions (admin)",
		},
	)

	access.RegisterEnforced(enforcer, "Billing",
		access.RoutePermission{
			Path: "/api/v1/invoices/user", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List user invoices",
		},
		access.RoutePermission{
			Path: "/api/v1/invoices/sync", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Sync invoices from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/invoices/:id/download", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Download invoice PDF",
		},
		access.RoutePermission{
			Path: "/api/v1/invoices/admin/cleanup", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Clean up orphaned invoices",
		},
		access.RoutePermission{
			Path: "/api/v1/payment-methods/user", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List user payment methods",
		},
		access.RoutePermission{
			Path: "/api/v1/payment-methods/sync", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Sync payment methods from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/payment-methods/:id/set-default", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Set default payment method",
		},
		access.RoutePermission{
			Path: "/api/v1/billing-addresses/user", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List user billing addresses",
		},
		access.RoutePermission{
			Path: "/api/v1/billing-addresses/:id/set-default", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Set default billing address",
		},
	)

	access.RegisterEnforced(enforcer, "Usage & Plans",
		access.RoutePermission{
			Path: "/api/v1/usage-metrics/user", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get user usage metrics",
		},
		access.RoutePermission{
			Path: "/api/v1/usage-metrics/increment", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Increment usage metric counter (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/usage-metrics/reset", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Reset usage metric counter (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-plans/:id/sync-stripe", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Sync single plan from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-plans/sync-stripe", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Sync all plans from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-plans/import-stripe", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Import plans from Stripe",
		},
		access.RoutePermission{
			Path: "/api/v1/hooks/stripe/toggle", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Toggle Stripe webhook processing",
		},
	)

	access.RegisterEnforced(enforcer, "Bulk Licenses",
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/create-checkout-session", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Create batch checkout session",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "List owned subscription batches",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Get subscription batch details",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/licenses", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "List licenses in a batch",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/assign", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Assign license from batch to user",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/licenses/:license_id/revoke", Method: "DELETE",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Revoke a batch license",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/quantity", Method: "PATCH",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Update batch license quantity",
		},
		access.RoutePermission{
			Path: "/api/v1/subscription-batches/:id/permanent", Method: "DELETE",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "SubscriptionBatch", Field: "PurchaserUserID"},
			Description: "Permanently delete a batch",
		},
	)

	log.Println("=== Payment module permissions registered ===")
}
