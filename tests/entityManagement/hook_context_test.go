// tests/entityManagement/hook_context_test.go
package entityManagement_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/entityManagement/hooks"
)

// ============================================================================
// Phase 0: HookContext UserID and UserRoles propagation tests
//
// These tests verify that:
// 1. EditEntity propagates UserID to BeforeUpdate/AfterUpdate hooks
// 2. EditEntityWithUser propagates UserID and UserRoles to hooks
// 3. HookContext.IsAdmin() correctly identifies administrator roles
// ============================================================================

// --- Test 1: EditEntity currently does NOT pass UserID to BeforeUpdate hooks ---
// This test proves the existing bug: EditEntity builds a HookContext but
// never sets UserID because its signature doesn't accept one.

func TestHookContext_EditEntity_UserIDIsEmpty(t *testing.T) {
	_, service, registry := setupHookServiceTestSimple(t)

	// Register a BeforeUpdate hook that captures the HookContext
	captureHook := NewSimpleTrackingHook(
		"capture-update-ctx",
		"HookTestEntity",
		[]hooks.HookType{hooks.BeforeUpdate},
		10,
	)
	err := registry.RegisterHook(captureHook)
	require.NoError(t, err)

	// Create an entity first
	entity := HookTestEntitySimple{
		Name:   "Original Name",
		Value:  42,
		Status: "active",
	}

	created, err := service.CreateEntityWithUser(entity, "HookTestEntity", "user-abc-123")
	require.NoError(t, err)

	createdEntity := created.(*HookTestEntitySimple)
	require.NotEqual(t, uuid.Nil, createdEntity.ID)

	// Now edit the entity — the current EditEntity does NOT accept a userID
	updateData := map[string]any{
		"name": "Updated Name",
	}

	entityModel := service.GetEntityModelInterface("HookTestEntity")
	err = service.EditEntity(createdEntity.ID, "HookTestEntity", entityModel, updateData)
	require.NoError(t, err)

	// Wait for hook execution
	time.Sleep(100 * time.Millisecond)

	// Verify the hook was called
	assert.Equal(t, 1, captureHook.GetExecutedCount(), "BeforeUpdate hook should have been called")

	// BUG: UserID is empty because EditEntity never sets it
	// After the fix, this assertion should be REMOVED and replaced by a non-empty check
	assert.Empty(t, captureHook.lastContext.UserID,
		"BUG: EditEntity does not propagate UserID to BeforeUpdate hook context. "+
			"After fix, UserID should be non-empty.")
}

// --- Test 1b: EditEntityWithUser should pass UserID to hooks ---
// This test will FAIL TO COMPILE because EditEntityWithUser does not exist yet.
// That compilation failure IS the expected TDD "Red" state.

func TestHookContext_EditEntityWithUser_PassesUserID(t *testing.T) {
	_, service, registry := setupHookServiceTestSimple(t)

	// Register a BeforeUpdate hook that captures the HookContext
	captureHook := NewSimpleTrackingHook(
		"capture-update-userid",
		"HookTestEntity",
		[]hooks.HookType{hooks.BeforeUpdate},
		10,
	)
	err := registry.RegisterHook(captureHook)
	require.NoError(t, err)

	// Create an entity first
	entity := HookTestEntitySimple{
		Name:   "Original Name",
		Value:  42,
		Status: "active",
	}

	created, err := service.CreateEntityWithUser(entity, "HookTestEntity", "user-abc-123")
	require.NoError(t, err)

	createdEntity := created.(*HookTestEntitySimple)

	// Edit using the new method that accepts userID
	updateData := map[string]any{
		"name": "Updated Name",
	}
	entityModel := service.GetEntityModelInterface("HookTestEntity")

	// EditEntityWithUser does not exist yet — this causes a compilation error (TDD Red)
	err = service.EditEntityWithUser(createdEntity.ID, "HookTestEntity", entityModel, updateData, "user-abc-123")
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, captureHook.GetExecutedCount(), "BeforeUpdate hook should have been called")
	assert.Equal(t, "user-abc-123", captureHook.lastContext.UserID,
		"EditEntityWithUser should propagate UserID to BeforeUpdate hook context")
}

// --- Test 2: EditEntityWithUser should pass UserRoles to hooks ---
// This test will FAIL TO COMPILE because:
//   - EditEntityWithUser does not exist yet
//   - HookContext.UserRoles field does not exist yet

func TestHookContext_EditEntityWithUser_PassesUserRoles(t *testing.T) {
	_, service, registry := setupHookServiceTestSimple(t)

	// Register a BeforeUpdate hook that captures the HookContext
	captureHook := NewSimpleTrackingHook(
		"capture-update-roles",
		"HookTestEntity",
		[]hooks.HookType{hooks.BeforeUpdate},
		10,
	)
	err := registry.RegisterHook(captureHook)
	require.NoError(t, err)

	// Create an entity first
	entity := HookTestEntitySimple{
		Name:   "Original Name",
		Value:  42,
		Status: "active",
	}

	created, err := service.CreateEntityWithUser(entity, "HookTestEntity", "user-abc-123")
	require.NoError(t, err)

	createdEntity := created.(*HookTestEntitySimple)

	// Edit using the new method that accepts userID and roles
	updateData := map[string]any{
		"name": "Updated Name",
	}
	entityModel := service.GetEntityModelInterface("HookTestEntity")

	// EditEntityWithUser does not exist yet — this causes a compilation error (TDD Red)
	err = service.EditEntityWithUser(createdEntity.ID, "HookTestEntity", entityModel, updateData, "user-abc-123")
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, captureHook.GetExecutedCount(), "BeforeUpdate hook should have been called")

	// UserRoles does not exist on HookContext yet — compilation error (TDD Red)
	assert.Contains(t, captureHook.lastContext.UserRoles, "Member",
		"EditEntityWithUser should propagate UserRoles to BeforeUpdate hook context")
}

// --- Test 3: IsAdmin returns true for administrator role ---
// This test will FAIL TO COMPILE because:
//   - HookContext.UserRoles field does not exist yet
//   - HookContext.IsAdmin() method does not exist yet

func TestHookContext_IsAdmin_ReturnsTrueForAdministrator(t *testing.T) {
	ctx := &hooks.HookContext{
		EntityName: "TestEntity",
		HookType:   hooks.BeforeUpdate,
		UserID:     "admin-user-1",
		UserRoles:  []string{"Administrator"},
	}

	assert.True(t, ctx.IsAdmin(),
		"IsAdmin() should return true when UserRoles contains 'Administrator'")
}

// --- Test 4: IsAdmin returns false for member role ---

func TestHookContext_IsAdmin_ReturnsFalseForMember(t *testing.T) {
	ctx := &hooks.HookContext{
		EntityName: "TestEntity",
		HookType:   hooks.BeforeUpdate,
		UserID:     "member-user-1",
		UserRoles:  []string{"Member"},
	}

	assert.False(t, ctx.IsAdmin(),
		"IsAdmin() should return false when UserRoles only contains 'Member'")
}

// --- Test 5: IsAdmin returns false for empty roles ---

func TestHookContext_IsAdmin_ReturnsFalseForEmptyRoles(t *testing.T) {
	ctx := &hooks.HookContext{
		EntityName: "TestEntity",
		HookType:   hooks.BeforeUpdate,
		UserID:     "user-no-roles",
		UserRoles:  []string{},
	}

	assert.False(t, ctx.IsAdmin(),
		"IsAdmin() should return false when UserRoles is empty")
}

// --- Test 5b: IsAdmin returns false for nil roles ---

func TestHookContext_IsAdmin_ReturnsFalseForNilRoles(t *testing.T) {
	ctx := &hooks.HookContext{
		EntityName: "TestEntity",
		HookType:   hooks.BeforeUpdate,
		UserID:     "user-nil-roles",
	}

	assert.False(t, ctx.IsAdmin(),
		"IsAdmin() should return false when UserRoles is nil")
}
