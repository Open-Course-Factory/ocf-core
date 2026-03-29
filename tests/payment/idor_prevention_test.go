// tests/payment/idor_prevention_test.go
//
// Security regression test: verifies that generic GET endpoints for payment
// entities containing sensitive user data are restricted to admin-only access.
//
// Background: The generic entity framework auto-generates REST routes
// (GET list, GET by ID, POST, PATCH, DELETE) based on the Roles map in each
// entity registration. Payment entities (BillingAddress, PaymentMethod, Invoice,
// UsageMetrics, UserSubscription) previously granted the Member role GET access,
// which means ANY authenticated user could list ALL users' billing addresses,
// payment methods, invoices, etc. via the generic /api/v1/<entity> endpoints.
//
// The fix removes GET from the Member role for these entities. Members access
// their own data through dedicated user-scoped routes (e.g.,
// /api/v1/user-subscriptions/current, /api/v1/billing-addresses/user) instead
// of the generic list/get-by-ID endpoints.
//
// This test calls the real registration functions and inspects the resulting
// Roles map to ensure Member does NOT have GET access to sensitive entities.
package payment_tests

import (
	"strings"
	"testing"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	registration "soli/formations/src/payment/entityRegistration"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sensitivePaymentEntities lists the entity names whose generic GET endpoints
// must be admin-only. These entities contain per-user billing and subscription
// data that should never be exposed to other users via generic list endpoints.
var sensitivePaymentEntities = []string{
	"BillingAddress",
	"PaymentMethod",
	"Invoice",
	"UsageMetrics",
	"UserSubscription",
}

// registerAllPaymentEntities creates a fresh EntityRegistrationService and
// registers all payment entities using the real registration functions.
// The Casbin enforcer is nil in tests, so setDefaultEntityAccesses is a no-op.
func registerAllPaymentEntities(t *testing.T) *ems.EntityRegistrationService {
	t.Helper()

	service := ems.NewEntityRegistrationService()

	registration.RegisterBillingAddress(service)
	registration.RegisterPaymentMethod(service)
	registration.RegisterInvoice(service)
	registration.RegisterUsageMetrics(service)
	registration.RegisterUserSubscription(service)

	return service
}

// TestSensitivePaymentEntities_MemberCannotGET verifies that the Member role
// does NOT have GET in its method regex for any sensitive payment entity.
//
// This test FAILS before the fix (Member has GET) and PASSES after.
func TestSensitivePaymentEntities_MemberCannotGET(t *testing.T) {
	service := registerAllPaymentEntities(t)
	allRoles := service.GetAllEntityRoles()

	memberRole := string(authModels.Member)

	for _, entityName := range sensitivePaymentEntities {
		t.Run(entityName+"_member_no_GET", func(t *testing.T) {
			entityRoles, exists := allRoles[entityName]
			require.True(t, exists,
				"Entity %s should be registered", entityName)

			memberMethods, hasMember := entityRoles.Roles[memberRole]

			if !hasMember {
				// Member role not defined at all = no access = safe
				return
			}

			assert.False(t, strings.Contains(memberMethods, "GET"),
				"SECURITY: Entity %s grants Member role GET access via generic endpoints "+
					"(methods: %q). This allows any authenticated user to read ALL users' "+
					"%s data through /api/v1/<entity>. Remove GET from Member role -- "+
					"members should use user-scoped routes instead.",
				entityName, memberMethods, entityName)
		})
	}
}

// TestSensitivePaymentEntities_AdminRetainsGET verifies that the Admin role
// still has GET access to all sensitive payment entities. Admins need to be
// able to view all data for support and management purposes.
func TestSensitivePaymentEntities_AdminRetainsGET(t *testing.T) {
	service := registerAllPaymentEntities(t)
	allRoles := service.GetAllEntityRoles()

	adminRole := string(authModels.Admin)

	for _, entityName := range sensitivePaymentEntities {
		t.Run(entityName+"_admin_has_GET", func(t *testing.T) {
			entityRoles, exists := allRoles[entityName]
			require.True(t, exists,
				"Entity %s should be registered", entityName)

			adminMethods, hasAdmin := entityRoles.Roles[adminRole]
			require.True(t, hasAdmin,
				"Entity %s should have an Admin role defined", entityName)

			assert.True(t, strings.Contains(adminMethods, "GET"),
				"Entity %s should grant Admin role GET access (methods: %q). "+
					"Admins need read access for platform management.",
				entityName, adminMethods)
		})
	}
}

// TestSensitivePaymentEntities_AllRegistered is a structural check ensuring
// that all expected sensitive entities are actually registered. If a new
// payment entity is added but not included in sensitivePaymentEntities,
// it could silently expose user data.
func TestSensitivePaymentEntities_AllRegistered(t *testing.T) {
	service := registerAllPaymentEntities(t)
	allRoles := service.GetAllEntityRoles()

	for _, entityName := range sensitivePaymentEntities {
		_, exists := allRoles[entityName]
		assert.True(t, exists,
			"Entity %s should be registered by the payment registration functions. "+
				"If it was renamed or removed, update sensitivePaymentEntities accordingly.",
			entityName)
	}
}
