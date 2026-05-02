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

	// Hook for gating scenario PATCH/DELETE on manageable scenarios
	scenarioAuthHook := NewScenarioAuthorizationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(scenarioAuthHook); err != nil {
		log.Printf("Failed to register scenario authorization hook: %v", err)
	} else {
		log.Println("Scenario authorization hook registered")
	}

	// Hook for verifying parent-scenario authorship before creating/updating/deleting steps
	stepAuthorizationHook := NewScenarioStepAuthorizationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(stepAuthorizationHook); err != nil {
		log.Printf("Failed to register scenario step authorization hook: %v", err)
	} else {
		log.Println("Scenario step authorization hook registered")
	}

	// Hook for verifying parent-scenario authorship before creating/updating/deleting step questions
	stepQuestionAuthorizationHook := NewScenarioStepQuestionAuthorizationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(stepQuestionAuthorizationHook); err != nil {
		log.Printf("Failed to register scenario step question authorization hook: %v", err)
	} else {
		log.Println("Scenario step question authorization hook registered")
	}

	log.Println("Scenario hooks initialization complete")
}
