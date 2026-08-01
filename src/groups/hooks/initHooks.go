package groupHooks

import (
	"log"
	"soli/formations/src/entityManagement/hooks"

	"gorm.io/gorm"
)

// InitGroupHooks registers all group-related hooks
func InitGroupHooks(db *gorm.DB) {
	log.Println("🔗 Initializing group hooks...")

	// Hook validating that a group may live in the organization it names.
	// Registered first because it gates the write.
	placementHook := NewGroupPlacementValidationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(placementHook); err != nil {
		log.Printf("❌ Failed to register group placement validation hook: %v", err)
	} else {
		log.Println("✅ Group placement validation hook registered")
	}

	// Hook deciding who may modify or delete an existing group. Registered
	// alongside the placement hook because both gate the write.
	writeAuthHook := NewGroupWriteAuthorizationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(writeAuthHook); err != nil {
		log.Printf("❌ Failed to register group write authorization hook: %v", err)
	} else {
		log.Println("✅ Group write authorization hook registered")
	}

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

	// Hook for validating group member addition
	memberValidationHook := NewGroupMemberValidationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(memberValidationHook); err != nil {
		log.Printf("❌ Failed to register group member validation hook: %v", err)
	} else {
		log.Println("✅ Group member validation hook registered")
	}

	// Hook for granting permissions when a member is added
	memberPermissionHook := NewGroupMemberPermissionHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(memberPermissionHook); err != nil {
		log.Printf("❌ Failed to register group member permission hook: %v", err)
	} else {
		log.Println("✅ Group member permission hook registered")
	}

	// Hook for revoking permissions when a member is removed
	memberCleanupHook := NewGroupMemberCleanupHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(memberCleanupHook); err != nil {
		log.Printf("❌ Failed to register group member cleanup hook: %v", err)
	} else {
		log.Println("✅ Group member cleanup hook registered")
	}

	// Hook for auto-assigning a license when a member is added to a group with a linked batch
	autoLicenseHook := NewGroupMemberAutoLicenseHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(autoLicenseHook); err != nil {
		log.Printf("❌ Failed to register group member auto-license hook: %v", err)
	} else {
		log.Println("✅ Group member auto-license hook registered")
	}

	log.Println("🔗 Group hooks initialization complete")
}
