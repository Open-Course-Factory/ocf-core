package authHooks

import (
	"log"

	access "soli/formations/src/auth/access"
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

	// Ownership hook to enforce that only the owner (or admin) can update UserSettings
	if err := hooks.GlobalHookRegistry.RegisterHook(hooks.NewOwnershipHook(db, "UserSettings", access.OwnershipConfig{
		OwnerField: "UserID", Operations: []string{"update"}, AdminBypass: true,
	})); err != nil {
		log.Printf("❌ Failed to register UserSettings ownership hook: %v", err)
	} else {
		log.Println("✅ UserSettings ownership hook registered")
	}

	log.Println("🔗 Auth hooks initialization complete")
}
