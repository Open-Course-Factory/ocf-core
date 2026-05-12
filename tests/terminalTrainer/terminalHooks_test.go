// tests/terminalTrainer/terminalHooks_test.go
//
// Failing tests locking the contract that AfterCreate / AfterDelete terminal
// hooks must NOT call enforcer.LoadPolicy() on the request hot path. The
// reload is ~100ms of latency per request for no consistency benefit (no
// Casbin Watcher is configured, so reloading is just re-reading what we
// already have in memory).
//
// The hook's actual job (AddPolicy / RemoveFilteredPolicy) MUST still happen
// — those assertions guard against an over-zealous "delete LoadPolicy and
// also accidentally delete the real call" refactor.
//
// Related: issue #321.
package terminalTrainer_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	"soli/formations/src/entityManagement/hooks"
	terminalHooks "soli/formations/src/terminalTrainer/hooks"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

func TestTerminalOwnerPermissionHook_DoesNotReloadPolicy(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()
	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = originalEnforcer }()

	terminal := &terminalModels.Terminal{UserID: "user-abc"}
	terminal.ID = uuid.New()

	hook := terminalHooks.NewTerminalOwnerPermissionHook(nil) // db unused by Execute
	err := hook.Execute(&hooks.HookContext{
		HookType:  hooks.AfterCreate,
		NewEntity: terminal,
	})

	assert.NoError(t, err)

	// CORE CONTRACT: no policy reload on the hot path.
	assert.Equal(t, 0, mockEnforcer.GetLoadPolicyCallCount(),
		"LoadPolicy must not be called from the terminal owner hook — each call is ~100ms request-path latency for no consistency benefit (no Casbin Watcher configured)")

	// BEHAVIOR PRESERVED: AddPolicy was actually called with the expected args.
	assert.Equal(t, 1, mockEnforcer.GetAddPolicyCallCount(),
		"AddPolicy must still be called once")

	if len(mockEnforcer.AddPolicyCalls) == 1 {
		call := mockEnforcer.AddPolicyCalls[0]
		assert.Equal(t, terminal.UserID, call[0], "subject should be terminal owner UserID")
		assert.Contains(t, call[1].(string), terminal.ID.String(), "route should contain terminal ID")
		assert.Equal(t, "PATCH", call[2], "method should be PATCH")
	}
}

func TestTerminalCleanupHook_DoesNotReloadPolicy(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()
	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = originalEnforcer }()

	terminal := &terminalModels.Terminal{UserID: "user-abc"}
	terminal.ID = uuid.New()

	hook := terminalHooks.NewTerminalCleanupHook(nil)
	err := hook.Execute(&hooks.HookContext{
		HookType:  hooks.AfterDelete,
		NewEntity: terminal,
	})

	assert.NoError(t, err)

	// CORE CONTRACT: no policy reload on the hot path.
	assert.Equal(t, 0, mockEnforcer.GetLoadPolicyCallCount(),
		"LoadPolicy must not be called from the terminal cleanup hook")

	// BEHAVIOR PRESERVED: RemoveFilteredPolicy was called.
	assert.Equal(t, 1, mockEnforcer.GetRemoveFilteredPolicyCallCount(),
		"RemoveFilteredPolicy must still be called once")

	if len(mockEnforcer.RemoveFilteredPolicyCalls) == 1 {
		call := mockEnforcer.RemoveFilteredPolicyCalls[0]
		// MockEnforcer records params as [fieldIndex, ...fieldValues]
		assert.Equal(t, 1, call[0], "fieldIndex should be 1 (route)")
		assert.Contains(t, call[1].(string), terminal.ID.String(), "route should contain terminal ID")
	}
}
