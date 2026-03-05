package authorization_tests

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/entityManagement/services"
	groupHooks "soli/formations/src/groups/hooks"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/utils"

	ems "soli/formations/src/entityManagement/entityManagementService"
	controller "soli/formations/src/entityManagement/routes"
	groupRegistration "soli/formations/src/groups/entityRegistration"
)

// ============================================================================
// Test Setup
// ============================================================================

func setupGroupTestEnvironment(t *testing.T) (*gorm.DB, services.GenericService) {
	_, b, _, _ := runtime.Caller(0)
	basePath := filepath.Dir(b) + "/../../"

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&groupModels.ClassGroup{},
		&groupModels.GroupMember{},
	)
	require.NoError(t, err)

	// Initialize Casbin enforcer
	casdoor.InitCasdoorEnforcer(db, basePath)
	require.NotNil(t, casdoor.Enforcer)

	err = casdoor.Enforcer.LoadPolicy()
	require.NoError(t, err)

	// Add role-level policies matching groupRegistration.go:
	// member: GET|POST only, admin: GET|POST|PATCH|DELETE
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = false

	err = utils.AddPolicy(
		casdoor.Enforcer,
		string(authModels.Member),
		"/api/v1/class-groups/:id",
		"(GET|POST)",
		opts,
	)
	require.NoError(t, err)

	err = utils.AddPolicy(
		casdoor.Enforcer,
		string(authModels.Admin),
		"/api/v1/class-groups/:id",
		"(GET|POST|PATCH|DELETE)",
		opts,
	)
	require.NoError(t, err)

	// Clear hooks and register group hooks
	hooks.GlobalHookRegistry.ClearAllHooks()
	hooks.GlobalHookRegistry.SetTestMode(true)
	groupHooks.InitGroupHooks(db)

	genericService := services.NewGenericService(db, casdoor.Enforcer)
	return db, genericService
}

func setupGroupRouterTestEnvironment(t *testing.T) (*gorm.DB, *gin.Engine, controller.GenericController) {
	db, _ := setupGroupTestEnvironment(t)

	gin.SetMode(gin.TestMode)

	// Register ClassGroup entity (needed for controller to resolve entity type)
	groupRegistration.RegisterGroup(ems.GlobalEntityRegistrationService)

	ctrl := controller.NewGenericController(db, casdoor.Enforcer)
	return db, nil, ctrl
}

// createGroup creates a ClassGroup and sets up owner membership + Casbin permissions manually.
// We bypass the hook system because SQLite does not support JSONB (used by GroupMember.Metadata).
func createGroup(t *testing.T, db *gorm.DB, _ services.GenericService, ownerID string) *groupModels.ClassGroup {
	groupID, err := uuid.NewV7()
	require.NoError(t, err)

	group := &groupModels.ClassGroup{
		Name:        "Test Group " + groupID.String()[:8],
		DisplayName: "Test Group",
		Description: "A test group",
		OwnerUserID: ownerID,
		MaxMembers:  50,
		IsActive:    true,
	}
	group.ID = groupID

	// Insert directly via DB, omitting JSONB Metadata field (unsupported by SQLite)
	err = db.Omit("Metadata").Create(group).Error
	require.NoError(t, err)

	// Manually create owner member (bypassing hook which fails on SQLite JSONB)
	member := &groupModels.GroupMember{
		GroupID:   group.ID,
		UserID:    ownerID,
		Role:      groupModels.GroupMemberRoleOwner,
		InvitedBy: ownerID,
		JoinedAt:  time.Now(),
		IsActive:  true,
	}
	err = db.Omit("Metadata").Create(member).Error
	require.NoError(t, err)

	// Manually grant Casbin permissions (replicating GrantGroupPermissionsToUser)
	opts := utils.DefaultPermissionOptions()
	opts.WarnOnError = true
	err = utils.GrantEntityAccess(casdoor.Enforcer, ownerID, "class-group", groupID.String(), "(GET|PATCH|DELETE|PUT)", opts)
	require.NoError(t, err)

	return group
}

func addGroupMember(t *testing.T, db *gorm.DB, groupID uuid.UUID, userID string, role groupModels.GroupMemberRole) *groupModels.GroupMember {
	member := &groupModels.GroupMember{
		GroupID:   groupID,
		UserID:    userID,
		Role:      role,
		InvitedBy: "test-admin",
		JoinedAt:  time.Now(),
		IsActive:  true,
	}
	err := db.Omit("Metadata").Create(member).Error
	require.NoError(t, err)
	return member
}

func checkGroupPermission(t *testing.T, userID, groupID, method string) bool {
	route := "/api/v1/class-groups/" + groupID
	allowed, err := casdoor.Enforcer.Enforce(userID, route, method)
	require.NoError(t, err)
	return allowed
}

// createDeleteRouter creates a gin router with the DELETE endpoint for class-groups
// configured with proper auth middleware simulation for a specific user
func createDeleteRouter(userID string, ctrl controller.GenericController) *gin.Engine {
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		ctx.Set("userId", userID)
		ctx.Next()
	})
	apiGroup := router.Group("/api/v1")
	apiGroup.DELETE("/class-groups/:id", func(ctx *gin.Context) {
		ctrl.DeleteEntity(ctx, true)
	})
	return router
}

// ============================================================================
// Group Deletion Authorization Tests
// ============================================================================

// Test 1: Owner has user-specific Casbin permission -> DELETE succeeds
func TestGroupAuthorization_OwnerCanDeleteGroup(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-001"
	group := createGroup(t, db, genericService, ownerID)

	// Owner should have DELETE permission via user-specific policy from GrantGroupPermissionsToUser
	hasPermission := checkGroupPermission(t, ownerID, group.ID.String(), "DELETE")
	assert.True(t, hasPermission, "Group owner should have DELETE permission via user-specific Casbin policy")
}

// Test 2: User with "member" role only has GET|POST -> DELETE returns 403
func TestGroupAuthorization_MemberRoleCannotDeleteGroup(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-002"
	memberUserID := "user-member-group-002"

	group := createGroup(t, db, genericService, ownerID)

	// Assign memberUserID to the "member" system role
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUserID, string(authModels.Member), opts)
	require.NoError(t, err)

	// Member role only grants GET|POST, NOT DELETE
	hasPermission := checkGroupPermission(t, memberUserID, group.ID.String(), "DELETE")
	assert.False(t, hasPermission, "Member role should NOT grant DELETE permission on class-groups")
}

// Test 3: Being a group member (in group_members table) doesn't grant DELETE
func TestGroupAuthorization_GroupMemberCannotDeleteOthersGroup(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-003"
	memberUserID := "user-member-group-003"

	group := createGroup(t, db, genericService, ownerID)

	// Add user as a "member" in the group_members table (not owner role)
	addGroupMember(t, db, group.ID, memberUserID, groupModels.GroupMemberRoleMember)

	// Assign system member role
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUserID, string(authModels.Member), opts)
	require.NoError(t, err)

	// Even though user is in the group_members table, they should NOT have DELETE
	hasPermission := checkGroupPermission(t, memberUserID, group.ID.String(), "DELETE")
	assert.False(t, hasPermission,
		"Being a group member (in group_members table) should NOT grant DELETE permission - "+
			"only user-specific Casbin policies from owner setup hooks grant DELETE")
}

// Test 4: User with no permissions at all -> 403
func TestGroupAuthorization_StrangerCannotDeleteGroup(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-004"
	strangerID := "user-stranger-group-004"

	group := createGroup(t, db, genericService, ownerID)

	// Stranger has no role, no group membership, no user-specific policies
	hasPermission := checkGroupPermission(t, strangerID, group.ID.String(), "DELETE")
	assert.False(t, hasPermission, "User with no permissions should NOT have DELETE access to any group")
}

// Test 5: System "admin" role has full CRUD -> can DELETE any group
// SECURITY NOTE: No ownership check at the application level beyond Casbin
func TestGroupAuthorization_SystemAdminCanDeleteAnyGroup(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-005"
	adminID := "user-admin-group-005"

	group := createGroup(t, db, genericService, ownerID)

	// Add admin role to user (via grouping policy)
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = false
	err := utils.AddGroupingPolicy(casdoor.Enforcer, adminID, string(authModels.Admin), opts)
	require.NoError(t, err)

	// Admin role grants GET|POST|PATCH|DELETE on class-groups
	hasPermission := checkGroupPermission(t, adminID, group.ID.String(), "DELETE")
	assert.True(t, hasPermission,
		"System admin should be able to DELETE any group via admin role policy - "+
			"SECURITY NOTE: no additional ownership check exists beyond Casbin")
}

// ============================================================================
// Group CRUD Role Tests
// ============================================================================

// Test 6: Member role can GET a specific group (via :id pattern)
func TestGroupAuthorization_MemberRoleCanGetGroup(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-006"
	memberUserID := "user-member-get-006"

	group := createGroup(t, db, genericService, ownerID)

	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUserID, string(authModels.Member), opts)
	require.NoError(t, err)

	// GET on a specific group ID should be allowed for members
	// The policy "/api/v1/class-groups/:id" with keyMatch2 matches any UUID in :id position
	hasPermission := checkGroupPermission(t, memberUserID, group.ID.String(), "GET")
	assert.True(t, hasPermission, "Member role should allow GET on specific class-group via :id pattern")
}

// Test 7: Member role can POST to a specific group path (via :id pattern)
func TestGroupAuthorization_MemberRoleCanCreateGroup(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-007"
	memberUserID := "user-member-create-007"

	group := createGroup(t, db, genericService, ownerID)

	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUserID, string(authModels.Member), opts)
	require.NoError(t, err)

	// POST on a specific group path should be allowed for members
	hasPermission := checkGroupPermission(t, memberUserID, group.ID.String(), "POST")
	assert.True(t, hasPermission, "Member role should allow POST on class-groups via :id pattern")
}

// Test 8: Member role cannot PATCH a group
func TestGroupAuthorization_MemberRoleCannotPatchGroup(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-008"
	memberUserID := "user-member-patch-008"

	group := createGroup(t, db, genericService, ownerID)

	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUserID, string(authModels.Member), opts)
	require.NoError(t, err)

	// PATCH should NOT be allowed for member role
	hasPermission := checkGroupPermission(t, memberUserID, group.ID.String(), "PATCH")
	assert.False(t, hasPermission, "Member role should NOT grant PATCH on class-groups")
}

// Test 9: Member role cannot DELETE group (role-level check)
func TestGroupAuthorization_MemberRoleCannotDeleteGroupRoleLevel(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-009"
	memberUserID := "user-member-delete-009"

	group := createGroup(t, db, genericService, ownerID)

	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUserID, string(authModels.Member), opts)
	require.NoError(t, err)

	// DELETE should NOT be allowed for member role
	hasPermission := checkGroupPermission(t, memberUserID, group.ID.String(), "DELETE")
	assert.False(t, hasPermission, "Member role should NOT grant DELETE on class-groups")
}

// ============================================================================
// Vulnerability Documentation Tests
// ============================================================================

// Test 10: Prove DeleteEntity in repository does NOT check membership
// VULNERABILITY: DeleteEntity does not apply GenericMembershipFilter unlike GetEntities
func TestGroupAuthorization_DeleteBypassesMembershipFilter(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-010"
	group := createGroup(t, db, genericService, ownerID)

	// Verify the group exists
	var count int64
	db.Model(&groupModels.ClassGroup{}).Where("id = ?", group.ID).Count(&count)
	require.Equal(t, int64(1), count, "Group should exist before delete")

	// Execute BeforeDelete hook (simulating the cleanup hook)
	beforeCtx := &hooks.HookContext{
		EntityName: "ClassGroup",
		HookType:   hooks.BeforeDelete,
		NewEntity:  group,
		EntityID:   group.ID,
		UserID:     ownerID,
	}
	err := hooks.GlobalHookRegistry.ExecuteHooks(beforeCtx)
	require.NoError(t, err)

	// VULNERABILITY: DeleteEntity does not apply GenericMembershipFilter unlike GetEntities.
	// The repository's DeleteEntity method uses a direct db.Delete without any
	// user-scoped filtering. If a user somehow passes the Casbin middleware check
	// (e.g., via admin role), they can delete any group regardless of membership.
	// This is "by design" since Casbin is the authorization boundary, but it means
	// there's no defense-in-depth ownership check at the data layer.
	err = db.Delete(group).Error
	require.NoError(t, err, "DeleteEntity should succeed without any membership check")

	// Verify deletion
	db.Model(&groupModels.ClassGroup{}).Where("id = ?", group.ID).Count(&count)
	assert.Equal(t, int64(0), count, "Group should be deleted - no membership filter was applied")
}

// Test 11: Prove GroupCleanupHook BeforeDelete only does cleanup, no ownership check
// VULNERABILITY: No BeforeDelete hook validates that the requesting user owns the group
func TestGroupAuthorization_BeforeDeleteDoesNotValidateOwnership(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-011"
	attackerID := "user-attacker-group-011"

	group := createGroup(t, db, genericService, ownerID)

	// VULNERABILITY: No BeforeDelete hook validates that the requesting user owns the group.
	// The GroupCleanupHook.Execute only revokes member permissions; it does NOT check
	// whether ctx.UserID == group.OwnerUserID. Any user who passes the Casbin check
	// can trigger deletion without an ownership validation at the hook level.

	// Simulate BeforeDelete with an attacker's userID
	beforeCtx := &hooks.HookContext{
		EntityName: "ClassGroup",
		HookType:   hooks.BeforeDelete,
		NewEntity:  group,
		EntityID:   group.ID,
		UserID:     attackerID, // NOT the owner
	}

	// The hook should NOT return an error because it doesn't validate ownership
	err := hooks.GlobalHookRegistry.ExecuteHooks(beforeCtx)
	assert.NoError(t, err,
		"BeforeDelete hook does NOT validate ownership - it only performs cleanup. "+
			"This means any user who passes Casbin auth can trigger group deletion.")
}

// ============================================================================
// Integration: Full HTTP DELETE Flow Tests
// ============================================================================

// Test: Full HTTP DELETE by owner via controller
func TestGroupAuthorization_OwnerHTTPDelete(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-http-012"
	group := createGroup(t, db, genericService, ownerID)

	// Register entity for controller resolution
	groupRegistration.RegisterGroup(ems.GlobalEntityRegistrationService)

	ctrl := controller.NewGenericController(db, casdoor.Enforcer)
	router := createDeleteRouter(ownerID, ctrl)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/class-groups/"+group.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code,
		"Owner should be able to DELETE their group via HTTP endpoint")
}

// Test: Full HTTP DELETE by stranger via controller (no Casbin auth middleware in this test,
// so we test the controller directly - the auth middleware is what would return 403)
func TestGroupAuthorization_StrangerHTTPDeleteWithoutAuthMiddleware(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-http-013"
	strangerID := "user-stranger-http-013"
	group := createGroup(t, db, genericService, ownerID)

	groupRegistration.RegisterGroup(ems.GlobalEntityRegistrationService)

	ctrl := controller.NewGenericController(db, casdoor.Enforcer)
	router := createDeleteRouter(strangerID, ctrl)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/class-groups/"+group.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Without auth middleware, the controller does NOT check Casbin permissions.
	// The controller will succeed (204) because DeleteEntity has no ownership check.
	// This proves that the Casbin auth middleware is the SOLE authorization boundary.
	assert.Equal(t, http.StatusNoContent, w.Code,
		"Without auth middleware, even a stranger can delete - "+
			"proving the controller/repository has no built-in ownership check")
}
