// tests/payment/userSubscription_crud_permissions_test.go
//
// Verifies that the UserSubscription entity registration restricts members to
// GET-only access on the generic CRUD routes (/api/v1/user-subscriptions).
//
// Members must NOT be able to POST or PATCH via generic CRUD, as that would
// bypass the checkout controller and Stripe validation. Write operations for
// members are only available through dedicated custom routes (checkout, cancel,
// upgrade, etc.) declared in permissions.go.
package payment_tests

import (
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/utils"
)

// crudPermBasePath returns the project root relative to this test file.
func crudPermBasePath() string {
	_, b, _, _ := runtime.Caller(0)
	return filepath.Dir(b) + "/../../"
}

// crudPermDBCounter generates unique in-memory DB names.
var crudPermDBCounter int

// setupCrudPermCasbinEnforcer creates an in-memory Casbin enforcer and loads
// the UserSubscription entity policies with the current registration roles.
func setupCrudPermCasbinEnforcer(t *testing.T) {
	t.Helper()

	crudPermDBCounter++
	dsn := fmt.Sprintf("file:crud_perm_test_%d?mode=memory&cache=shared&_busy_timeout=5000", crudPermDBCounter)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	casdoor.InitCasdoorEnforcer(db, crudPermBasePath())
	require.NotNil(t, casdoor.Enforcer)

	err = casdoor.Enforcer.LoadPolicy()
	require.NoError(t, err)

	// Register the UserSubscription entity policies using the ACTUAL roles
	// from the registration (member: GET only, admin: all methods).
	service := ems.NewEntityRegistrationService()
	roles := entityManagementInterfaces.EntityRoles{
		Roles: map[string]string{
			string(authModels.Member): "(" + http.MethodGet + ")",
			string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")",
		},
	}
	service.SetDefaultEntityAccesses("UserSubscription", roles, casdoor.Enforcer)
}

// ============================================================================
// Member CRUD access — generic routes must be GET-only
// ============================================================================

func TestUserSubscriptionCRUD_MemberCanGET(t *testing.T) {
	setupCrudPermCasbinEnforcer(t)

	memberUser := "member-crud-get-ok"
	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUser, string(authModels.Member), opts)
	require.NoError(t, err)

	// List endpoint
	allowed, err := casdoor.Enforcer.Enforce(memberUser, "/api/v1/user-subscriptions", "GET")
	require.NoError(t, err)
	assert.True(t, allowed, "Member should be allowed GET on /api/v1/user-subscriptions (list)")

	// Single resource endpoint
	allowed, err = casdoor.Enforcer.Enforce(memberUser, "/api/v1/user-subscriptions/550e8400-e29b-41d4-a716-446655440000", "GET")
	require.NoError(t, err)
	assert.True(t, allowed, "Member should be allowed GET on /api/v1/user-subscriptions/:id")
}

func TestUserSubscriptionCRUD_MemberDeniedPOST(t *testing.T) {
	setupCrudPermCasbinEnforcer(t)

	memberUser := "member-crud-post-denied"
	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUser, string(authModels.Member), opts)
	require.NoError(t, err)

	// List endpoint (POST creates a new entity)
	allowed, err := casdoor.Enforcer.Enforce(memberUser, "/api/v1/user-subscriptions", "POST")
	require.NoError(t, err)
	assert.False(t, allowed,
		"Member MUST be denied POST on /api/v1/user-subscriptions — "+
			"subscriptions must go through the checkout controller with Stripe validation")
}

func TestUserSubscriptionCRUD_MemberDeniedPATCH(t *testing.T) {
	setupCrudPermCasbinEnforcer(t)

	memberUser := "member-crud-patch-denied"
	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUser, string(authModels.Member), opts)
	require.NoError(t, err)

	// Resource endpoint (PATCH updates an entity)
	allowed, err := casdoor.Enforcer.Enforce(memberUser, "/api/v1/user-subscriptions/550e8400-e29b-41d4-a716-446655440000", "PATCH")
	require.NoError(t, err)
	assert.False(t, allowed,
		"Member MUST be denied PATCH on /api/v1/user-subscriptions/:id — "+
			"subscription modifications must go through dedicated endpoints (cancel, upgrade)")
}

func TestUserSubscriptionCRUD_MemberDeniedDELETE(t *testing.T) {
	setupCrudPermCasbinEnforcer(t)

	memberUser := "member-crud-delete-denied"
	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUser, string(authModels.Member), opts)
	require.NoError(t, err)

	allowed, err := casdoor.Enforcer.Enforce(memberUser, "/api/v1/user-subscriptions/550e8400-e29b-41d4-a716-446655440000", "DELETE")
	require.NoError(t, err)
	assert.False(t, allowed,
		"Member MUST be denied DELETE on /api/v1/user-subscriptions/:id")
}

// ============================================================================
// Admin CRUD access — admin should have full access
// ============================================================================

func TestUserSubscriptionCRUD_AdminHasFullAccess(t *testing.T) {
	setupCrudPermCasbinEnforcer(t)

	adminUser := "admin-crud-full-access"
	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, adminUser, string(authModels.Admin), opts)
	require.NoError(t, err)

	methods := []string{"GET", "POST", "PATCH", "DELETE"}
	paths := []string{
		"/api/v1/user-subscriptions",
		"/api/v1/user-subscriptions/550e8400-e29b-41d4-a716-446655440000",
	}

	for _, method := range methods {
		for _, path := range paths {
			allowed, err := casdoor.Enforcer.Enforce(adminUser, path, method)
			require.NoError(t, err)
			assert.True(t, allowed,
				"Admin should be allowed %s on %s", method, path)
		}
	}
}
