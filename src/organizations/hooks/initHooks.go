package organizationHooks

import (
	"log"
	"soli/formations/src/entityManagement/hooks"

	"gorm.io/gorm"
)

// InitOrganizationHooks registers all organization-related hooks
func InitOrganizationHooks(db *gorm.DB) {
	log.Println("🔗 Initializing organization hooks...")

	// Hook for setting up organization owner and creating owner member
	ownerSetupHook := NewOrganizationOwnerSetupHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(ownerSetupHook); err != nil {
		log.Printf("❌ Failed to register organization owner setup hook: %v", err)
	} else {
		log.Println("✅ Organization owner setup hook registered")
	}

	// Hook for cleaning up permissions when an organization is deleted
	cleanupHook := NewOrganizationCleanupHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(cleanupHook); err != nil {
		log.Printf("❌ Failed to register organization cleanup hook: %v", err)
	} else {
		log.Println("✅ Organization cleanup hook registered")
	}

	// Hook for validating organization member addition
	memberValidationHook := NewOrganizationMemberValidationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(memberValidationHook); err != nil {
		log.Printf("❌ Failed to register organization member validation hook: %v", err)
	} else {
		log.Println("✅ Organization member validation hook registered")
	}

	// Hook for validating organization member deletion
	memberDeletionHook := NewOrganizationMemberDeletionHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(memberDeletionHook); err != nil {
		log.Printf("❌ Failed to register organization member deletion hook: %v", err)
	} else {
		log.Println("✅ Organization member deletion hook registered")
	}

	log.Println("🔗 Organization hooks initialization complete")
}
