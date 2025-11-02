package initialization

import (
	"log"
	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
	"strings"
)

// SetupPaymentRolePermissions sets up role-based permissions for payment system
func SetupPaymentRolePermissions(enforcer interfaces.EnforcerInterface) {
	enforcer.LoadPolicy()

	// Add permissions for /api/v1/users/:id (which includes /api/v1/users/me)
	// Get all Casdoor roles that map to the "member" OCF role
	casdoorRoles := authModels.GetCasdoorRolesForOCFRole(authModels.Member)

	for _, role := range casdoorRoles {
		// Add permission for /api/v1/users/:id (the actual route pattern)
		_, err1 := enforcer.AddPolicy(role, "/api/v1/users/:id", "GET")
		if err1 != nil {
			if strings.Contains(err1.Error(), "UNIQUE") {
				log.Printf("Permission already exists: %s /api/v1/users/:id", role)
			} else {
				log.Printf("Error adding permission for %s /api/v1/users/:id: %v", role, err1)
			}
		} else {
			log.Printf("✅ Added %s permission for /api/v1/users/:id (includes /me)", role)
		}

		// Add permission for /api/v1/users/me/* sub-paths
		_, err2 := enforcer.AddPolicy(role, "/api/v1/users/me/*", "(GET|POST|PATCH|DELETE)")
		if err2 != nil {
			if strings.Contains(err2.Error(), "UNIQUE") {
				log.Printf("Permission already exists: %s /api/v1/users/me/*", role)
			} else {
				log.Printf("Error adding permission for %s /api/v1/users/me/*: %v", role, err2)
			}
		} else {
			log.Printf("✅ Added %s permission for /api/v1/users/me/*", role)
		}

		// Add permission for /api/v1/auth/permissions (new permissions endpoint)
		_, err3 := enforcer.AddPolicy(role, "/api/v1/auth/permissions", "GET")
		if err3 != nil {
			if strings.Contains(err3.Error(), "UNIQUE") {
				log.Printf("Permission already exists: %s /api/v1/auth/permissions", role)
			} else {
				log.Printf("Error adding permission for %s /api/v1/auth/permissions: %v", role, err3)
			}
		} else {
			log.Printf("✅ Added %s permission for /api/v1/auth/permissions", role)
		}
	}

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
