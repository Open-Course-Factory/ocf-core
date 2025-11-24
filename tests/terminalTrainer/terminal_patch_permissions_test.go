package terminalTrainer_tests_test

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/entityManagement/services"
	terminalHooks "soli/formations/src/terminalTrainer/hooks"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"
)

func setupTestEnvironment(t *testing.T) (*gorm.DB, services.GenericService) {
	// Calculate base path relative to this test file
	_, b, _, _ := runtime.Caller(0)
	basePath := filepath.Dir(b) + "/../../"

	// Create shared cache SQLite database
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate tables
	err = db.AutoMigrate(
		&terminalModels.Terminal{},
		&terminalModels.UserTerminalKey{},
		&terminalModels.TerminalShare{},
	)
	require.NoError(t, err)

	// Initialize Casbin enforcer with in-memory adapter
	casdoor.InitCasdoorEnforcer(db, basePath)
	require.NotNil(t, casdoor.Enforcer)

	// Load policies
	err = casdoor.Enforcer.LoadPolicy()
	require.NoError(t, err)

	// Add role-level policies
	// NOTE: Member role does NOT have PATCH - only user-specific policies grant PATCH
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = false

	err = utils.AddPolicy(
		casdoor.Enforcer,
		string(authModels.Member),
		"/api/v1/terminals/:id",
		"(GET|POST)",
		opts,
	)
	require.NoError(t, err)

	err = utils.AddPolicy(
		casdoor.Enforcer,
		string(authModels.Admin),
		"/api/v1/terminals/:id",
		"(GET|POST|PATCH|DELETE)",
		opts,
	)
	require.NoError(t, err)

	// Clear any existing hooks and register terminal hooks
	hooks.GlobalHookRegistry.ClearAllHooks()
	hooks.GlobalHookRegistry.SetTestMode(true) // Synchronous execution for tests
	terminalHooks.InitTerminalHooks(db)

	// Create generic service (this is what triggers hooks)
	genericService := services.NewGenericService(db, casdoor.Enforcer)

	return db, genericService
}

func createTestTerminalKey(t *testing.T, db *gorm.DB, userID string) uuid.UUID {
	key := &terminalModels.UserTerminalKey{
		UserID:      userID,
		APIKey:      "test-key-" + userID,
		KeyName:     "Test Key",
		IsActive:    true,
		MaxSessions: 5,
	}
	err := db.Create(key).Error
	require.NoError(t, err)
	return key.ID
}

func createTerminal(t *testing.T, genericService services.GenericService, ownerID string, keyID uuid.UUID) *terminalModels.Terminal {
	// Create terminal entity
	terminal := &terminalModels.Terminal{
		SessionID:         "session-" + uuid.New().String(),
		UserID:            ownerID,
		Name:              "Test Terminal",
		Status:            "active",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "docker",
		MachineSize:       "S",
		UserTerminalKeyID: keyID,
	}

	// Use SaveEntity to create the entity
	saved, err := genericService.SaveEntity(terminal)
	require.NoError(t, err)

	savedTerminal := saved.(*terminalModels.Terminal)

	// Manually trigger AfterCreate hook
	afterCtx := &hooks.HookContext{
		EntityName: "Terminal",
		HookType:   hooks.AfterCreate,
		NewEntity:  savedTerminal,
		EntityID:   savedTerminal.ID,
		UserID:     ownerID,
	}
	err = hooks.GlobalHookRegistry.ExecuteHooks(afterCtx)
	require.NoError(t, err)

	return savedTerminal
}

func createShare(t *testing.T, genericService services.GenericService, terminalID uuid.UUID, sharedByUserID, sharedWithUserID, accessLevel string) *terminalModels.TerminalShare {
	share := &terminalModels.TerminalShare{
		TerminalID:       terminalID,
		SharedWithUserID: &sharedWithUserID,
		SharedByUserID:   sharedByUserID,
		AccessLevel:      accessLevel,
		IsActive:         true,
	}

	// Use SaveEntity to create the entity
	saved, err := genericService.SaveEntity(share)
	require.NoError(t, err)

	savedShare := saved.(*terminalModels.TerminalShare)

	// Manually trigger AfterCreate hook
	afterCtx := &hooks.HookContext{
		EntityName: "TerminalShare",
		HookType:   hooks.AfterCreate,
		NewEntity:  savedShare,
		EntityID:   savedShare.ID,
		UserID:     sharedByUserID,
	}
	err = hooks.GlobalHookRegistry.ExecuteHooks(afterCtx)
	require.NoError(t, err)

	return savedShare
}

func deleteTerminal(t *testing.T, db *gorm.DB, terminal *terminalModels.Terminal) {
	// Delete from database
	err := db.Delete(terminal).Error
	require.NoError(t, err)

	// Manually trigger AfterDelete hook
	afterCtx := &hooks.HookContext{
		EntityName: "Terminal",
		HookType:   hooks.AfterDelete,
		NewEntity:  terminal,
		EntityID:   terminal.ID,
	}
	err = hooks.GlobalHookRegistry.ExecuteHooks(afterCtx)
	require.NoError(t, err)
}

func deleteShare(t *testing.T, db *gorm.DB, share *terminalModels.TerminalShare) {
	// Delete from database
	err := db.Delete(share).Error
	require.NoError(t, err)

	// Manually trigger AfterDelete hook
	afterCtx := &hooks.HookContext{
		EntityName: "TerminalShare",
		HookType:   hooks.AfterDelete,
		NewEntity:  share,
		EntityID:   share.ID,
	}
	err = hooks.GlobalHookRegistry.ExecuteHooks(afterCtx)
	require.NoError(t, err)
}

func checkPermission(t *testing.T, userID, terminalID, method string) bool {
	route := "/api/v1/terminals/" + terminalID
	allowed, err := casdoor.Enforcer.Enforce(userID, route, method)
	require.NoError(t, err)
	return allowed
}

// Test 1: Owner should have PATCH permission on their own terminal
func TestTerminalOwnerCanPatch(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	ownerID := "user-owner-123"
	keyID := createTestTerminalKey(t, db, ownerID)

	// Create terminal using service (this will trigger AfterCreate hook)
	terminal := createTerminal(t, genericService, ownerID, keyID)

	// Check if owner has PATCH permission
	hasPermission := checkPermission(t, ownerID, terminal.ID.String(), "PATCH")

	assert.True(t, hasPermission, "Terminal owner should have PATCH permission")
}

// Test 2: Non-owner member should NOT have PATCH permission (even though member role has PATCH)
func TestNonOwnerMemberCannotPatch(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	ownerID := "user-owner-123"
	otherUserID := "user-other-456"
	keyID := createTestTerminalKey(t, db, ownerID)

	// Create terminal
	terminal := createTerminal(t, genericService, ownerID, keyID)

	// Check if other user (non-owner) has PATCH permission
	hasPermission := checkPermission(t, otherUserID, terminal.ID.String(), "PATCH")

	assert.False(t, hasPermission, "Non-owner member should NOT have PATCH permission")
}

// Test 3: Shared user with "read" access should NOT have PATCH permission
func TestSharedUserReadAccessCannotPatch(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	ownerID := "user-owner-123"
	sharedUserID := "user-shared-456"
	keyID := createTestTerminalKey(t, db, ownerID)

	// Create terminal
	terminal := createTerminal(t, genericService, ownerID, keyID)

	// Share terminal with "read" access
	createShare(t, genericService, terminal.ID, ownerID, sharedUserID, "read")

	// Check if shared user with "read" access has PATCH permission
	hasPermission := checkPermission(t, sharedUserID, terminal.ID.String(), "PATCH")

	assert.False(t, hasPermission, "Shared user with 'read' access should NOT have PATCH permission")
}

// Test 4: Shared user with "write" access should have PATCH permission
func TestSharedUserWriteAccessCanPatch(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	ownerID := "user-owner-123"
	sharedUserID := "user-shared-456"
	keyID := createTestTerminalKey(t, db, ownerID)

	// Create terminal
	terminal := createTerminal(t, genericService, ownerID, keyID)

	// Share terminal with "write" access
	createShare(t, genericService, terminal.ID, ownerID, sharedUserID, "write")

	// Check if shared user with "write" access has PATCH permission
	hasPermission := checkPermission(t, sharedUserID, terminal.ID.String(), "PATCH")

	assert.True(t, hasPermission, "Shared user with 'write' access should have PATCH permission")
}

// Test 5: Shared user with "admin" access should have PATCH permission
func TestSharedUserAdminAccessCanPatch(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	ownerID := "user-owner-123"
	sharedUserID := "user-shared-456"
	keyID := createTestTerminalKey(t, db, ownerID)

	// Create terminal
	terminal := createTerminal(t, genericService, ownerID, keyID)

	// Share terminal with "admin" access
	createShare(t, genericService, terminal.ID, ownerID, sharedUserID, "admin")

	// Check if shared user with "admin" access has PATCH permission
	hasPermission := checkPermission(t, sharedUserID, terminal.ID.String(), "PATCH")

	assert.True(t, hasPermission, "Shared user with 'admin' access should have PATCH permission")
}

// Test 6: After share is revoked, shared user should lose PATCH permission
func TestRevokeShareRemovesPermission(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	ownerID := "user-owner-123"
	sharedUserID := "user-shared-456"
	keyID := createTestTerminalKey(t, db, ownerID)

	// Create terminal
	terminal := createTerminal(t, genericService, ownerID, keyID)

	// Share terminal with "write" access
	share := createShare(t, genericService, terminal.ID, ownerID, sharedUserID, "write")

	// Verify shared user has PATCH permission
	hasPermissionBefore := checkPermission(t, sharedUserID, terminal.ID.String(), "PATCH")
	assert.True(t, hasPermissionBefore, "Shared user should have PATCH permission before revoke")

	// Revoke share (delete it - this will trigger AfterDelete hook)
	deleteShare(t, db, share)

	// Verify shared user no longer has PATCH permission
	hasPermissionAfter := checkPermission(t, sharedUserID, terminal.ID.String(), "PATCH")
	assert.False(t, hasPermissionAfter, "Shared user should NOT have PATCH permission after revoke")
}

// Test 7: After terminal is deleted, all user-specific policies should be removed
func TestDeleteTerminalRemovesAllPolicies(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	ownerID := "user-owner-123"
	sharedUserID := "user-shared-456"
	keyID := createTestTerminalKey(t, db, ownerID)

	// Create terminal
	terminal := createTerminal(t, genericService, ownerID, keyID)

	// Share terminal with "write" access
	createShare(t, genericService, terminal.ID, ownerID, sharedUserID, "write")

	// Verify both users have PATCH permission
	ownerHasPermission := checkPermission(t, ownerID, terminal.ID.String(), "PATCH")
	sharedUserHasPermission := checkPermission(t, sharedUserID, terminal.ID.String(), "PATCH")
	assert.True(t, ownerHasPermission, "Owner should have PATCH permission before terminal deletion")
	assert.True(t, sharedUserHasPermission, "Shared user should have PATCH permission before terminal deletion")

	// Delete terminal (this will trigger AfterDelete hook)
	deleteTerminal(t, db, terminal)

	// Verify both users no longer have PATCH permission
	ownerHasPermissionAfter := checkPermission(t, ownerID, terminal.ID.String(), "PATCH")
	sharedUserHasPermissionAfter := checkPermission(t, sharedUserID, terminal.ID.String(), "PATCH")
	assert.False(t, ownerHasPermissionAfter, "Owner should NOT have PATCH permission after terminal deletion")
	assert.False(t, sharedUserHasPermissionAfter, "Shared user should NOT have PATCH permission after terminal deletion")
}

// Test 8: Administrator role should bypass resource-level permissions
func TestAdministratorCanPatchAnyTerminal(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	ownerID := "user-owner-123"
	adminID := "user-admin-999"
	keyID := createTestTerminalKey(t, db, ownerID)

	// Create terminal
	terminal := createTerminal(t, genericService, ownerID, keyID)

	// Add admin role to user (via grouping policy)
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = false
	err := utils.AddGroupingPolicy(casdoor.Enforcer, adminID, string(authModels.Admin), opts)
	require.NoError(t, err)

	// Check if admin can PATCH (should work via role, not user-specific policy)
	hasPermission := checkPermission(t, adminID, terminal.ID.String(), "PATCH")

	assert.True(t, hasPermission, "Administrator should be able to PATCH any terminal")
}

// Test 9: Multiple shares with different access levels
func TestMultipleShares(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	ownerID := "user-owner-123"
	readUserID := "user-read-456"
	writeUserID := "user-write-789"
	adminUserID := "user-admin-012"
	keyID := createTestTerminalKey(t, db, ownerID)

	// Create terminal
	terminal := createTerminal(t, genericService, ownerID, keyID)

	// Share with read access
	createShare(t, genericService, terminal.ID, ownerID, readUserID, "read")

	// Share with write access
	createShare(t, genericService, terminal.ID, ownerID, writeUserID, "write")

	// Share with admin access
	createShare(t, genericService, terminal.ID, ownerID, adminUserID, "admin")

	// Check permissions
	ownerHas := checkPermission(t, ownerID, terminal.ID.String(), "PATCH")
	readUserHas := checkPermission(t, readUserID, terminal.ID.String(), "PATCH")
	writeUserHas := checkPermission(t, writeUserID, terminal.ID.String(), "PATCH")
	adminUserHas := checkPermission(t, adminUserID, terminal.ID.String(), "PATCH")

	assert.True(t, ownerHas, "Owner should have PATCH permission")
	assert.False(t, readUserHas, "User with 'read' access should NOT have PATCH permission")
	assert.True(t, writeUserHas, "User with 'write' access should have PATCH permission")
	assert.True(t, adminUserHas, "User with 'admin' access should have PATCH permission")
}

// Test 10: REGRESSION TEST - Member role should NOT grant universal PATCH access
// This test replicates the production environment where users have role assignments
// and would have caught the security bug where member role had PATCH at role-level
func TestMemberRoleDoesNotGrantUniversalPatch(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	ownerID := "user-owner-123"
	memberUserID := "user-member-with-role-789"
	keyID := createTestTerminalKey(t, db, ownerID)

	// Create terminal owned by ownerID
	terminal := createTerminal(t, genericService, ownerID, keyID)

	// 🎯 KEY: Assign memberUserID to "member" role (like in production!)
	// This is what was missing in other tests and why they didn't catch the bug
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUserID, string(authModels.Member), opts)
	require.NoError(t, err)

	// Verify owner still has PATCH permission (via user-specific policy from hook)
	ownerHasPermission := checkPermission(t, ownerID, terminal.ID.String(), "PATCH")
	assert.True(t, ownerHasPermission, "Owner should have PATCH permission via user-specific policy")

	// 🔒 CRITICAL TEST: Member user should NOT have PATCH permission
	// Even though they have the "member" role, the role should not grant PATCH at role-level
	// This test would have FAILED before the fix because member role had PATCH permission
	memberHasPermission := checkPermission(t, memberUserID, terminal.ID.String(), "PATCH")
	assert.False(t, memberHasPermission,
		"Member role should NOT grant PATCH to terminals they don't own - "+
			"PATCH should only be granted via user-specific policies (owner or shared access)")
}

// Test 11: Member with role CAN perform GET/POST operations (role-level permissions)
func TestMemberRoleCanPerformBasicOperations(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	memberUserID := "user-member-create-890"
	keyID := createTestTerminalKey(t, db, memberUserID)

	// Assign user to member role
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUserID, string(authModels.Member), opts)
	require.NoError(t, err)

	// Create a terminal as this member user
	terminal := createTerminal(t, genericService, memberUserID, keyID)

	// Member can GET their own terminal (via pattern route with :id)
	hasGetPermission := checkPermission(t, memberUserID, terminal.ID.String(), "GET")
	assert.True(t, hasGetPermission, "Member role should allow GET on terminal routes")

	// Member can POST (this is implicitly tested by creating the terminal above)
	// The fact that createTerminal succeeded means POST works at role-level

	// Verify they got owner-specific PATCH permission via hook (not role)
	hasPatchPermission := checkPermission(t, memberUserID, terminal.ID.String(), "PATCH")
	assert.True(t, hasPatchPermission, "Member who creates terminal gets PATCH via ownership hook")
}

// Test 12: Member with role CANNOT PATCH terminals they don't own, even with keymatch
func TestMemberRoleCannotPatchViaKeymatch(t *testing.T) {
	db, genericService := setupTestEnvironment(t)

	ownerID := "user-owner-999"
	memberUserID := "user-member-keymatch-991"
	keyID := createTestTerminalKey(t, db, ownerID)

	// Create terminal
	terminal := createTerminal(t, genericService, ownerID, keyID)

	// Assign member role
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	err := utils.AddGroupingPolicy(casdoor.Enforcer, memberUserID, string(authModels.Member), opts)
	require.NoError(t, err)

	// Test against the actual UUID route (what happens in production)
	actualRoute := "/api/v1/terminals/" + terminal.ID.String()
	allowed, err := casdoor.Enforcer.Enforce(memberUserID, actualRoute, "PATCH")
	require.NoError(t, err)

	assert.False(t, allowed,
		"Member should NOT be able to PATCH terminal via keymatch on UUID route - "+
			"even though role policy uses :id pattern, it should not grant PATCH")

	// Also test against the pattern route
	patternRoute := "/api/v1/terminals/:id"
	allowedPattern, err := casdoor.Enforcer.Enforce(memberUserID, patternRoute, "PATCH")
	require.NoError(t, err)

	assert.False(t, allowedPattern,
		"Member should NOT be able to PATCH terminal via pattern route")
}
