package scenarioHooks

import (
	"log"
	"soli/formations/src/entityManagement/hooks"

	"gorm.io/gorm"
)

// InitScenarioHooks registers all scenario-related hooks
func InitScenarioHooks(db *gorm.DB) {
	log.Println("Initializing scenario hooks...")

	// Hook for verifying group ownership before creating/deleting scenario assignments
	authorizationHook := NewScenarioAssignmentAuthorizationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(authorizationHook); err != nil {
		log.Printf("Failed to register scenario assignment authorization hook: %v", err)
	} else {
		log.Println("Scenario assignment authorization hook registered")
	}

	log.Println("Scenario hooks initialization complete")
}
