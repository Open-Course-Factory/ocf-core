package groupHooks

import (
	"log"
	"soli/formations/src/entityManagement/hooks"

	"gorm.io/gorm"
)

// InitGroupHooks registers all group-related hooks
func InitGroupHooks(db *gorm.DB) {
	log.Println("🔗 Initializing group hooks...")

	// Hook for setting up group owner and creating owner member
	ownerSetupHook := NewGroupOwnerSetupHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(ownerSetupHook); err != nil {
		log.Printf("❌ Failed to register group owner setup hook: %v", err)
	} else {
		log.Println("✅ Group owner setup hook registered")
	}

	// Hook for cleaning up permissions when a group is deleted
	cleanupHook := NewGroupCleanupHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(cleanupHook); err != nil {
		log.Printf("❌ Failed to register group cleanup hook: %v", err)
	} else {
		log.Println("✅ Group cleanup hook registered")
	}

	log.Println("🔗 Group hooks initialization complete")
}
