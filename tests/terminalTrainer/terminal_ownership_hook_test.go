// tests/terminalTrainer/terminal_ownership_hook_test.go
package terminalTrainer_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	access "soli/formations/src/auth/access"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	terminalHooks "soli/formations/src/terminalTrainer/hooks"
	"soli/formations/src/terminalTrainer/models"
)

var terminalOwnershipConfig = access.OwnershipConfig{
	OwnerField: "UserID", Operations: []string{"create"}, AdminBypass: true,
}

// ============================================================================
// Terminal Ownership Hook — BeforeCreate Tests
// ============================================================================

func TestTerminalOwnership_BeforeCreate_SetsUserID(t *testing.T) {
	db := setupTestDB(t)
	hook := hooks.NewOwnershipHook(db, "Terminal", terminalOwnershipConfig)

	userID := "user-creator-123"
	terminal := &models.Terminal{
		SessionID: "test-session-1",
		Name:      "My Terminal",
	}

	ctx := &hooks.HookContext{
		EntityName: "Terminal",
		HookType:   hooks.BeforeCreate,
		NewEntity:  terminal,
		UserID:     userID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err)
	assert.Equal(t, userID, terminal.UserID,
		"BeforeCreate should set UserID from authenticated user")
}

func TestTerminalOwnership_BeforeCreate_PreventsImpersonation(t *testing.T) {
	db := setupTestDB(t)
	hook := hooks.NewOwnershipHook(db, "Terminal", terminalOwnershipConfig)

	authenticatedUserID := "real-user-123"
	terminal := &models.Terminal{
		SessionID: "test-session-2",
		UserID:    "attacker-spoofed-id", // Attacker tries to set someone else's ID
		Name:      "Spoofed Terminal",
	}

	ctx := &hooks.HookContext{
		EntityName: "Terminal",
		HookType:   hooks.BeforeCreate,
		NewEntity:  terminal,
		UserID:     authenticatedUserID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err)
	assert.Equal(t, authenticatedUserID, terminal.UserID,
		"BeforeCreate should override spoofed UserID with authenticated user's ID")
}

func TestTerminalOwnership_BeforeCreate_AdminCanSetAnyUserID(t *testing.T) {
	db := setupTestDB(t)
	hook := hooks.NewOwnershipHook(db, "Terminal", terminalOwnershipConfig)

	adminID := "admin-user-789"
	targetUserID := "target-user-456"
	terminal := &models.Terminal{
		SessionID: "test-session-3",
		UserID:    targetUserID,
		Name:      "Admin-Created Terminal",
	}

	ctx := &hooks.HookContext{
		EntityName: "Terminal",
		HookType:   hooks.BeforeCreate,
		NewEntity:  terminal,
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err)
	assert.Equal(t, targetUserID, terminal.UserID,
		"Admin should be able to set any UserID")
}

// ============================================================================
// TerminalShare Ownership Hook — BeforeCreate Tests
// ============================================================================

func TestTerminalShareOwnership_BeforeCreate_SetsSharedByUserID(t *testing.T) {
	db := setupTestDB(t)
	hook := terminalHooks.NewTerminalShareOwnershipHook(db)

	ownerID := "user-owner-123"
	recipientID := "user-recipient-456"

	// Create a terminal owned by the user
	userKey, err := createTestUserKey(db, ownerID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerID, "active", userKey.ID)
	require.NoError(t, err)

	share := &models.TerminalShare{
		TerminalID:       terminal.ID,
		SharedWithUserID: &recipientID,
		AccessLevel:      models.AccessLevelRead,
	}

	ctx := &hooks.HookContext{
		EntityName: "TerminalShare",
		HookType:   hooks.BeforeCreate,
		NewEntity:  share,
		UserID:     ownerID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.NoError(t, err)
	assert.Equal(t, ownerID, share.SharedByUserID,
		"BeforeCreate should set SharedByUserID from authenticated user")
}

func TestTerminalShareOwnership_BeforeCreate_OwnerCanShare(t *testing.T) {
	db := setupTestDB(t)
	hook := terminalHooks.NewTerminalShareOwnershipHook(db)

	ownerID := "user-owner-123"
	recipientID := "user-recipient-456"

	// Create a terminal owned by the user
	userKey, err := createTestUserKey(db, ownerID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerID, "active", userKey.ID)
	require.NoError(t, err)

	share := &models.TerminalShare{
		TerminalID:       terminal.ID,
		SharedWithUserID: &recipientID,
		AccessLevel:      models.AccessLevelRead,
	}

	ctx := &hooks.HookContext{
		EntityName: "TerminalShare",
		HookType:   hooks.BeforeCreate,
		NewEntity:  share,
		UserID:     ownerID,
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.NoError(t, err, "Terminal owner should be able to create a share")
}

func TestTerminalShareOwnership_BeforeCreate_NonOwnerBlocked(t *testing.T) {
	db := setupTestDB(t)
	hook := terminalHooks.NewTerminalShareOwnershipHook(db)

	ownerID := "user-owner-123"
	attackerID := "user-attacker-456"
	recipientID := "user-recipient-789"

	// Create a terminal owned by ownerID
	userKey, err := createTestUserKey(db, ownerID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerID, "active", userKey.ID)
	require.NoError(t, err)

	share := &models.TerminalShare{
		TerminalID:       terminal.ID,
		SharedWithUserID: &recipientID,
		AccessLevel:      models.AccessLevelRead,
	}

	ctx := &hooks.HookContext{
		EntityName: "TerminalShare",
		HookType:   hooks.BeforeCreate,
		NewEntity:  share,
		UserID:     attackerID, // Not the terminal owner
		UserRoles:  []string{"Member"},
	}

	err = hook.Execute(ctx)
	assert.Error(t, err, "Non-owner should be blocked from sharing someone else's terminal")
	assert.Contains(t, err.Error(), "permission",
		"Error should mention permission denial")
}

func TestTerminalShareOwnership_BeforeCreate_AdminCanShareAny(t *testing.T) {
	db := setupTestDB(t)
	hook := terminalHooks.NewTerminalShareOwnershipHook(db)

	ownerID := "user-owner-123"
	adminID := "admin-user-789"
	recipientID := "user-recipient-456"

	// Create a terminal owned by ownerID
	userKey, err := createTestUserKey(db, ownerID)
	require.NoError(t, err)
	terminal, err := createTestTerminal(db, ownerID, "active", userKey.ID)
	require.NoError(t, err)

	share := &models.TerminalShare{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		TerminalID:       terminal.ID,
		SharedWithUserID: &recipientID,
		SharedByUserID:   adminID,
		AccessLevel:      models.AccessLevelRead,
	}

	ctx := &hooks.HookContext{
		EntityName: "TerminalShare",
		HookType:   hooks.BeforeCreate,
		NewEntity:  share,
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err = hook.Execute(ctx)
	assert.NoError(t, err, "Admin should be able to share any terminal")
}

func TestTerminalShareOwnership_BeforeCreate_TerminalNotFound(t *testing.T) {
	db := setupTestDB(t)
	hook := terminalHooks.NewTerminalShareOwnershipHook(db)

	userID := "user-123"
	recipientID := "user-recipient-456"
	nonExistentTerminalID := uuid.New()

	share := &models.TerminalShare{
		TerminalID:       nonExistentTerminalID,
		SharedWithUserID: &recipientID,
		AccessLevel:      models.AccessLevelRead,
	}

	ctx := &hooks.HookContext{
		EntityName: "TerminalShare",
		HookType:   hooks.BeforeCreate,
		NewEntity:  share,
		UserID:     userID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Should fail when terminal does not exist")
	assert.Contains(t, err.Error(), "terminal not found",
		"Error should indicate terminal was not found")
}
