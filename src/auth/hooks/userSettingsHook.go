package authHooks

import (
	"log"
	"soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/hooks"

	"gorm.io/gorm"
)

type UserSettingsHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewUserSettingsHook(db *gorm.DB) hooks.Hook {
	return &UserSettingsHook{
		db:       db,
		enabled:  true,
		priority: 10, // Normal priority
	}
}

func (h *UserSettingsHook) GetName() string {
	return "user_settings_auto_create"
}

func (h *UserSettingsHook) GetEntityName() string {
	// This hook doesn't target a specific entity in the entity management system
	// Instead, it's triggered by external user creation events
	return "User"
}

func (h *UserSettingsHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{
		hooks.AfterCreate,
	}
}

func (h *UserSettingsHook) IsEnabled() bool {
	return h.enabled
}

func (h *UserSettingsHook) GetPriority() int {
	return h.priority
}

func (h *UserSettingsHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType != hooks.AfterCreate {
		return nil
	}

	return h.handleAfterUserCreate(ctx)
}

func (h *UserSettingsHook) handleAfterUserCreate(ctx *hooks.HookContext) error {
	// This hook is designed to be called manually from user creation code
	// The user ID should be available in the metadata
	userID, ok := ctx.Metadata["user_id"].(string)
	if !ok || userID == "" {
		log.Printf("⚠️  UserSettingsHook: No user_id in metadata")
		return nil // Don't fail, just log
	}

	// Check if settings already exist (idempotency)
	var existingSettings models.UserSettings
	result := h.db.Where("user_id = ?", userID).First(&existingSettings)
	if result.Error == nil {
		log.Printf("✅ UserSettings already exist for user %s", userID)
		return nil // Settings already exist, no need to create
	}

	// Create default settings
	defaultSettings := models.UserSettings{
		UserID:               userID,
		DefaultLandingPage:   "/dashboard",
		PreferredLanguage:    "en",
		Timezone:             "UTC",
		Theme:                "light",
		CompactMode:          false,
		EmailNotifications:   true,
		DesktopNotifications: false,
		TwoFactorEnabled:     false,
	}

	if err := h.db.Create(&defaultSettings).Error; err != nil {
		log.Printf("❌ Failed to create default settings for user %s: %v", userID, err)
		return err
	}

	log.Printf("✅ Created default settings for user %s", userID)
	return nil
}

// Helper function to call from user creation code
func CreateDefaultSettingsForUser(db *gorm.DB, userID string) error {
	// Check if settings already exist
	var existingSettings models.UserSettings
	result := db.Where("user_id = ?", userID).First(&existingSettings)
	if result.Error == nil {
		return nil // Settings already exist
	}

	// Create default settings
	defaultSettings := models.UserSettings{
		UserID:               userID,
		DefaultLandingPage:   "/dashboard",
		PreferredLanguage:    "en",
		Timezone:             "UTC",
		Theme:                "light",
		CompactMode:          false,
		EmailNotifications:   true,
		DesktopNotifications: false,
		TwoFactorEnabled:     false,
	}

	return db.Create(&defaultSettings).Error
}
