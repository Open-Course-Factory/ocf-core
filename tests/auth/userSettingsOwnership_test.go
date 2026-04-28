// tests/auth/userSettingsOwnership_test.go
package auth_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	access "soli/formations/src/auth/access"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
)

var userSettingsOwnershipConfig = access.OwnershipConfig{
	OwnerField: "UserID", Operations: []string{"update"}, AdminBypass: true,
}

// setupUserSettingsTestDB creates an in-memory SQLite DB with UserSettings table
func setupUserSettingsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	err = db.AutoMigrate(&authModels.UserSettings{})
	require.NoError(t, err)
	return db
}

// createTestUserSettings inserts a UserSettings record owned by the given userID
func createTestUserSettings(t *testing.T, db *gorm.DB, userID string) *authModels.UserSettings {
	t.Helper()
	settings := &authModels.UserSettings{
		BaseModel:         entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:            userID,
		DefaultLandingPage: "/dashboard",
		PreferredLanguage: "en",
		Timezone:          "UTC",
		Theme:             "light",
	}
	err := db.Create(settings).Error
	require.NoError(t, err)
	return settings
}

// ============================================================================
// UserSettings Ownership Hook — BeforeUpdate Tests
// (Member can only GET and PATCH per Casbin, so only BeforeUpdate is needed)
// ============================================================================

func TestUserSettingsOwnership_BeforeUpdate_OwnerCanUpdate(t *testing.T) {
	db := setupUserSettingsTestDB(t)
	hook := hooks.NewOwnershipHook(db, "UserSetting", userSettingsOwnershipConfig)

	ownerID := "user-owner-123"
	settings := createTestUserSettings(t, db, ownerID)

	// Owner updates their own settings
	ctx := &hooks.HookContext{
		EntityName: "UserSetting",
		HookType:   hooks.BeforeUpdate,
		EntityID:   settings.ID,
		OldEntity:  settings,
		NewEntity:  map[string]any{"theme": "dark"},
		UserID:     ownerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Owner should be able to update their own settings")
}

func TestUserSettingsOwnership_BeforeUpdate_NonOwnerBlocked(t *testing.T) {
	db := setupUserSettingsTestDB(t)
	hook := hooks.NewOwnershipHook(db, "UserSetting", userSettingsOwnershipConfig)

	ownerID := "user-owner-123"
	attackerID := "user-attacker-456"
	settings := createTestUserSettings(t, db, ownerID)

	// Non-owner tries to update someone else's settings
	ctx := &hooks.HookContext{
		EntityName: "UserSetting",
		HookType:   hooks.BeforeUpdate,
		EntityID:   settings.ID,
		OldEntity:  settings,
		NewEntity:  map[string]any{"theme": "dark"},
		UserID:     attackerID,
		UserRoles:  []string{"Member"},
	}

	err := hook.Execute(ctx)
	assert.Error(t, err, "Non-owner should be blocked from updating settings")
	assert.Contains(t, err.Error(), "permission", "Error should mention permission denial")
}

func TestUserSettingsOwnership_BeforeUpdate_AdminCanUpdate(t *testing.T) {
	db := setupUserSettingsTestDB(t)
	hook := hooks.NewOwnershipHook(db, "UserSetting", userSettingsOwnershipConfig)

	ownerID := "user-owner-123"
	adminID := "admin-user-789"
	settings := createTestUserSettings(t, db, ownerID)

	// Admin updates someone else's settings
	ctx := &hooks.HookContext{
		EntityName: "UserSetting",
		HookType:   hooks.BeforeUpdate,
		EntityID:   settings.ID,
		OldEntity:  settings,
		NewEntity:  map[string]any{"theme": "dark"},
		UserID:     adminID,
		UserRoles:  []string{"Administrator"},
	}

	err := hook.Execute(ctx)
	assert.NoError(t, err, "Admin should be able to update any user's settings")
}
