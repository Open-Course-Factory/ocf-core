// tests/entityManagement/ownership_hook_factory_test.go
package entityManagement_tests

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
	"soli/formations/src/entityManagement/hooks"
)

// ============================================================================
// Test entity for ownership hook factory tests
// ============================================================================

// TestOwnedEntity is a minimal entity with a UserID field for ownership checks.
type TestOwnedEntity struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID string    `gorm:"type:varchar(255)"`
	Name   string
}

// setupOwnershipTestDB creates an in-memory SQLite DB with TestOwnedEntity migrated.
func setupOwnershipTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&TestOwnedEntity{})
	require.NoError(t, err)

	return db
}

// insertTestOwnedEntity inserts a TestOwnedEntity and returns it with a generated UUID.
func insertTestOwnedEntity(t *testing.T, db *gorm.DB, userID, name string) TestOwnedEntity {
	t.Helper()

	id, err := uuid.NewV7()
	require.NoError(t, err)

	entity := TestOwnedEntity{
		ID:     id,
		UserID: userID,
		Name:   name,
	}

	result := db.Create(&entity)
	require.NoError(t, result.Error)

	return entity
}

// ============================================================================
// Phase 3: Generic Ownership Hook Factory Tests (TDD Red)
//
// These tests call hooks.NewOwnershipHook which does NOT exist yet.
// They also reference casbin.OwnershipConfig which does NOT exist yet.
// Expected: compile failure on both symbols.
// ============================================================================

// --- Test 1: BeforeCreate forces UserID from context ---

func TestOwnershipHook_BeforeCreate_ForcesUserID(t *testing.T) {
	db := setupOwnershipTestDB(t)

	config := access.OwnershipConfig{
		OwnerField:  "UserID",
		Operations:  []string{"create", "update", "delete"},
		AdminBypass: true,
	}

	hook := hooks.NewOwnershipHook(db, "TestOwnedEntity", config)
	require.NotNil(t, hook)

	entity := TestOwnedEntity{
		UserID: "", // Empty — hook should force it to ctx.UserID
		Name:   "Test Entity",
	}

	ctx := &hooks.HookContext{
		EntityName: "TestOwnedEntity",
		HookType:   hooks.BeforeCreate,
		UserID:     "user1",
		UserRoles:  []string{"member"},
		NewEntity:  &entity,
		Context:    context.Background(),
	}

	err := hook.Execute(ctx)
	require.NoError(t, err)

	assert.Equal(t, "user1", entity.UserID,
		"BeforeCreate should force entity.UserID to the authenticated user's ID")
}

// --- Test 2: BeforeCreate admin bypass (admin can create for another user) ---

func TestOwnershipHook_BeforeCreate_AdminBypass(t *testing.T) {
	db := setupOwnershipTestDB(t)

	config := access.OwnershipConfig{
		OwnerField:  "UserID",
		Operations:  []string{"create", "update", "delete"},
		AdminBypass: true,
	}

	hook := hooks.NewOwnershipHook(db, "TestOwnedEntity", config)
	require.NotNil(t, hook)

	entity := TestOwnedEntity{
		UserID: "other-user", // Admin is creating for someone else
		Name:   "Admin Created Entity",
	}

	ctx := &hooks.HookContext{
		EntityName: "TestOwnedEntity",
		HookType:   hooks.BeforeCreate,
		UserID:     "admin1",
		UserRoles:  []string{"administrator"},
		NewEntity:  &entity,
		Context:    context.Background(),
	}

	err := hook.Execute(ctx)
	require.NoError(t, err)

	assert.Equal(t, "other-user", entity.UserID,
		"BeforeCreate with admin should NOT override entity.UserID — admin can create for anyone")
}

// --- Test 3: BeforeUpdate allows owner ---

func TestOwnershipHook_BeforeUpdate_OwnerAllowed(t *testing.T) {
	db := setupOwnershipTestDB(t)

	entity := insertTestOwnedEntity(t, db, "user1", "My Entity")

	config := access.OwnershipConfig{
		OwnerField:  "UserID",
		Operations:  []string{"create", "update", "delete"},
		AdminBypass: true,
	}

	hook := hooks.NewOwnershipHook(db, "TestOwnedEntity", config)
	require.NotNil(t, hook)

	ctx := &hooks.HookContext{
		EntityName: "TestOwnedEntity",
		HookType:   hooks.BeforeUpdate,
		UserID:     "user1",
		UserRoles:  []string{"member"},
		EntityID:   entity.ID.String(),
		NewEntity:  &entity,
		Context:    context.Background(),
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Owner should be allowed to update their own entity")
}

// --- Test 4: BeforeUpdate denies non-owner ---

func TestOwnershipHook_BeforeUpdate_NonOwnerDenied(t *testing.T) {
	db := setupOwnershipTestDB(t)

	entity := insertTestOwnedEntity(t, db, "user1", "User1 Entity")

	config := access.OwnershipConfig{
		OwnerField:  "UserID",
		Operations:  []string{"create", "update", "delete"},
		AdminBypass: true,
	}

	hook := hooks.NewOwnershipHook(db, "TestOwnedEntity", config)
	require.NotNil(t, hook)

	ctx := &hooks.HookContext{
		EntityName: "TestOwnedEntity",
		HookType:   hooks.BeforeUpdate,
		UserID:     "user2", // Different user — should be denied
		UserRoles:  []string{"member"},
		EntityID:   entity.ID.String(),
		NewEntity:  &entity,
		Context:    context.Background(),
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Non-owner should be denied from updating another user's entity")
	assert.Contains(t, err.Error(), "permission",
		"Error should indicate a permission denial")
}

// --- Test 5: BeforeUpdate admin bypass ---

func TestOwnershipHook_BeforeUpdate_AdminBypass(t *testing.T) {
	db := setupOwnershipTestDB(t)

	entity := insertTestOwnedEntity(t, db, "user1", "User1 Entity")

	config := access.OwnershipConfig{
		OwnerField:  "UserID",
		Operations:  []string{"create", "update", "delete"},
		AdminBypass: true,
	}

	hook := hooks.NewOwnershipHook(db, "TestOwnedEntity", config)
	require.NotNil(t, hook)

	ctx := &hooks.HookContext{
		EntityName: "TestOwnedEntity",
		HookType:   hooks.BeforeUpdate,
		UserID:     "user2",
		UserRoles:  []string{"administrator"}, // Admin can update anyone's entity
		EntityID:   entity.ID.String(),
		NewEntity:  &entity,
		Context:    context.Background(),
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Admin should bypass ownership check on update")
}

// --- Test 6: BeforeDelete allows owner ---

func TestOwnershipHook_BeforeDelete_OwnerAllowed(t *testing.T) {
	db := setupOwnershipTestDB(t)

	entity := insertTestOwnedEntity(t, db, "user1", "My Entity")

	config := access.OwnershipConfig{
		OwnerField:  "UserID",
		Operations:  []string{"create", "update", "delete"},
		AdminBypass: true,
	}

	hook := hooks.NewOwnershipHook(db, "TestOwnedEntity", config)
	require.NotNil(t, hook)

	ctx := &hooks.HookContext{
		EntityName: "TestOwnedEntity",
		HookType:   hooks.BeforeDelete,
		UserID:     "user1",
		UserRoles:  []string{"member"},
		EntityID:   entity.ID.String(),
		NewEntity:  &entity,
		Context:    context.Background(),
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Owner should be allowed to delete their own entity")
}

// --- Test 7: BeforeDelete denies non-owner ---

func TestOwnershipHook_BeforeDelete_NonOwnerDenied(t *testing.T) {
	db := setupOwnershipTestDB(t)

	entity := insertTestOwnedEntity(t, db, "user1", "User1 Entity")

	config := access.OwnershipConfig{
		OwnerField:  "UserID",
		Operations:  []string{"create", "update", "delete"},
		AdminBypass: true,
	}

	hook := hooks.NewOwnershipHook(db, "TestOwnedEntity", config)
	require.NotNil(t, hook)

	ctx := &hooks.HookContext{
		EntityName: "TestOwnedEntity",
		HookType:   hooks.BeforeDelete,
		UserID:     "user2", // Different user — should be denied
		UserRoles:  []string{"member"},
		EntityID:   entity.ID.String(),
		NewEntity:  &entity,
		Context:    context.Background(),
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Non-owner should be denied from deleting another user's entity")
	assert.Contains(t, err.Error(), "permission",
		"Error should indicate a permission denial")
}
