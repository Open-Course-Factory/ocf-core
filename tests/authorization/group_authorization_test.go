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

	// Clear hooks and register group hooks
	hooks.GlobalHookRegistry.ClearAllHooks()
	groupHooks.InitGroupHooks(db)

	genericService := services.NewGenericService(db, casdoor.Enforcer)
	return db, genericService
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
// Write authorization hook
// ============================================================================

// A user who cannot manage the group is refused by the BeforeDelete hook.
func TestGroupAuthorization_BeforeDeleteRejectsUsersWhoCannotManageTheGroup(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-011"
	attackerID := "user-attacker-group-011"

	group := createGroup(t, db, genericService, ownerID)

	// This test used to assert the OPPOSITE, documenting a hole: no BeforeDelete
	// hook validated who was deleting, so anyone past the Casbin gate could
	// destroy any group. Casbin only decides which METHODS a role may use, never
	// which row — so with DELETE granted to Member (#459) that was the whole
	// authorization story.
	//
	// GroupWriteAuthorizationHook now answers "may this user change this group",
	// via GroupService.CanUserManageGroup: the group's owner, a group manager, or
	// a manager of the owning organization.
	beforeCtx := &hooks.HookContext{
		EntityName: "ClassGroup",
		HookType:   hooks.BeforeDelete,
		NewEntity:  group,
		EntityID:   group.ID,
		UserID:     attackerID, // NOT the owner, not a member, no org role
	}

	err := hooks.GlobalHookRegistry.ExecuteHooks(beforeCtx)

	assert.Error(t, err,
		"a user who cannot manage the group must not be able to delete it — "+
			"before-hooks gate the write, and this is the only row-level check there is")
}

// The owner is still allowed: a gate that refuses everyone is not a fix.
func TestGroupAuthorization_BeforeDeleteAllowsTheOwner(t *testing.T) {
	db, genericService := setupGroupTestEnvironment(t)

	ownerID := "user-owner-group-011b"
	group := createGroup(t, db, genericService, ownerID)

	beforeCtx := &hooks.HookContext{
		EntityName: "ClassGroup",
		HookType:   hooks.BeforeDelete,
		NewEntity:  group,
		EntityID:   group.ID,
		UserID:     ownerID,
	}

	assert.NoError(t, hooks.GlobalHookRegistry.ExecuteHooks(beforeCtx),
		"the group's owner must still be able to delete it")
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
func TestGroupAuthorization_StrangerHTTPDeleteIsRefusedWithoutAuthMiddleware(t *testing.T) {
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

	// This asserted 204 before #459, documenting that Casbin was the SOLE
	// authorization boundary — remove the middleware and a stranger could delete
	// anything. That was tolerable only while Member lacked DELETE entirely.
	//
	// Now the write path carries its own row-level check, so the refusal survives
	// without the middleware. Defense in depth, not a replacement for it: the
	// middleware still decides whether the method is reachable at all.
	assert.Equal(t, http.StatusForbidden, w.Code,
		"a stranger must be refused by the write path itself, not only by the auth middleware")

	var count int64
	db.Model(&groupModels.ClassGroup{}).Where("id = ?", group.ID).Count(&count)
	assert.Equal(t, int64(1), count, "the group must survive a refused delete")
}
