package initialization

import (
	"soli/formations/src/auth/interfaces"
)

// SetupPaymentRolePermissions sets up role-based permissions for payment system
func SetupPaymentRolePermissions(enforcer interfaces.EnforcerInterface) {
	enforcer.LoadPolicy()

	enforcer.AddPolicy("member", "/api/v1/users/me/*", "(GET|POST|PATCH|DELETE)")

	// Permissions pour Student Premium
	enforcer.AddPolicy("member_pro", "/api/v1/terminals/*", "(GET|POST)")
	enforcer.AddPolicy("member_pro", "/api/v1/user-subscriptions/current", "GET")
	enforcer.AddPolicy("member_pro", "/api/v1/user-subscriptions/portal", "POST")
	enforcer.AddPolicy("member_pro", "/api/v1/invoices/user", "GET")
	enforcer.AddPolicy("member_pro", "/api/v1/payment-methods/user", "GET")

	// Permissions pour Organization (hérite de supervisor_pro)
	enforcer.AddPolicy("organization", "/api/v1/*", "(GET|POST|PATCH|DELETE)")
	enforcer.AddPolicy("organization", "/api/v1/users/*", "(GET|POST|PATCH)")
	enforcer.AddPolicy("organization", "/api/v1/groups/*", "(GET|POST|PATCH|DELETE)")

	// Groupements de rôles (hiérarchie)
	enforcer.AddGroupingPolicy("member_pro", "member")
	enforcer.AddGroupingPolicy("organization", "member_pro")
}
