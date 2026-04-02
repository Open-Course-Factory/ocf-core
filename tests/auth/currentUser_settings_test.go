package auth_tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"soli/formations/src/auth/dto"
	authModels "soli/formations/src/auth/models"
	userController "soli/formations/src/auth/routes/usersRoutes"
)

// setupCurrentUserTestDB creates an in-memory SQLite DB with UserSettings table
func setupCurrentUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	err = db.AutoMigrate(&authModels.UserSettings{})
	require.NoError(t, err)
	return db
}

// --- DTO structure test ---

func TestCurrentUserOutput_IncludesSettingsField(t *testing.T) {
	// Verify the DTO has a settings field that serializes correctly
	settings := &dto.UserSettingsOutput{
		Theme:             "dark",
		PreferredLanguage: "fr",
	}
	output := &dto.CurrentUserOutput{
		UserID:      "test-user-1",
		UserName:    "testuser",
		DisplayName: "Test User",
		Email:       "test@example.com",
		Roles:       []string{"member"},
		Settings:    settings,
	}

	jsonBytes, err := json.Marshal(output)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	// Verify settings field is present in JSON output
	settingsField, ok := parsed["settings"]
	require.True(t, ok, "settings field must be present in JSON output")
	settingsMap, ok := settingsField.(map[string]any)
	require.True(t, ok, "settings must be an object")
	assert.Equal(t, "dark", settingsMap["theme"])
	assert.Equal(t, "fr", settingsMap["preferred_language"])
}

func TestCurrentUserOutput_SettingsOmittedWhenNil(t *testing.T) {
	output := &dto.CurrentUserOutput{
		UserID:   "test-user-2",
		UserName: "testuser2",
		Roles:    []string{"member"},
		Settings: nil,
	}

	jsonBytes, err := json.Marshal(output)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	// Settings should be omitted when nil (omitempty)
	_, ok := parsed["settings"]
	assert.False(t, ok, "settings field should be omitted when nil")
}

// --- FetchUserSettings tests ---

func TestFetchUserSettings_CreatesDefaultsForNewUser(t *testing.T) {
	db := setupCurrentUserTestDB(t)

	settings, err := userController.FetchUserSettings(db, "new-user-123")
	require.NoError(t, err)
	require.NotNil(t, settings)

	assert.Equal(t, "new-user-123", settings.UserID)
	assert.Equal(t, "/dashboard", settings.DefaultLandingPage)
	assert.Equal(t, "en", settings.PreferredLanguage)
	assert.Equal(t, "UTC", settings.Timezone)
	assert.Equal(t, "light", settings.Theme)
	assert.False(t, settings.CompactMode)
	assert.True(t, settings.EmailNotifications)
	assert.False(t, settings.DesktopNotifications)
	assert.False(t, settings.TwoFactorEnabled)
	assert.Nil(t, settings.PasswordLastChanged)
}

func TestFetchUserSettings_ReturnsExistingSettings(t *testing.T) {
	db := setupCurrentUserTestDB(t)

	// Pre-create settings with custom values
	existing := authModels.UserSettings{
		UserID:             "existing-user-456",
		DefaultLandingPage: "/courses",
		PreferredLanguage:  "fr",
		Timezone:           "Europe/Paris",
		Theme:              "dark",
		CompactMode:        true,
		EmailNotifications: true,
	}
	require.NoError(t, db.Create(&existing).Error)

	settings, err := userController.FetchUserSettings(db, "existing-user-456")
	require.NoError(t, err)
	require.NotNil(t, settings)

	assert.Equal(t, "existing-user-456", settings.UserID)
	assert.Equal(t, "/courses", settings.DefaultLandingPage)
	assert.Equal(t, "fr", settings.PreferredLanguage)
	assert.Equal(t, "Europe/Paris", settings.Timezone)
	assert.Equal(t, "dark", settings.Theme)
	assert.True(t, settings.CompactMode)
	assert.True(t, settings.EmailNotifications)
}

func TestFetchUserSettings_IdempotentForSameUser(t *testing.T) {
	db := setupCurrentUserTestDB(t)

	// Call twice — should create once, then return the same record
	settings1, err := userController.FetchUserSettings(db, "idempotent-user")
	require.NoError(t, err)

	settings2, err := userController.FetchUserSettings(db, "idempotent-user")
	require.NoError(t, err)

	assert.Equal(t, settings1.ID, settings2.ID, "should return the same settings record")
	assert.Equal(t, settings1.Theme, settings2.Theme)

	// Only one row should exist
	var count int64
	db.Model(&authModels.UserSettings{}).Where("user_id = ?", "idempotent-user").Count(&count)
	assert.Equal(t, int64(1), count)
}
