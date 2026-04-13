// tests/terminalTrainer/terminal_ownership_hook_test.go
package terminalTrainer_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
	"soli/formations/src/entityManagement/hooks"
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

