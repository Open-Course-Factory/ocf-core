package authHooks

import (
	"log"
	"soli/formations/src/entityManagement/hooks"

	"gorm.io/gorm"
)

// InitAuthHooks registers all authentication-related hooks
func InitAuthHooks(db *gorm.DB) {
	log.Println("🔗 Initializing auth hooks...")

	// Hook to automatically create default settings when a user is created
	userSettingsHook := NewUserSettingsHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(userSettingsHook); err != nil {
		log.Printf("❌ Failed to register UserSettings hook: %v", err)
	} else {
		log.Println("✅ UserSettings auto-create hook registered")
	}

	// Future hooks can be added here:
	// - Welcome email hook
	// - Initial permissions setup hook
	// - User analytics tracking hook
	// etc.

	log.Println("🔗 Auth hooks initialization complete")
}
