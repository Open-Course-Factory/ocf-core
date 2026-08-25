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
	archivedHook := NewScenarioAssignmentArchivedHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(archivedHook); err != nil {
		log.Printf("Failed to register scenario assignment archived hook: %v", err)
	} else {
		log.Println("Scenario assignment archived hook registered")
	}

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

	// Translations are Member-writable, so every write operation on them needs
	// an authorization hook. Without one the entity is not weakly protected but
	// unprotected, and any learner could rewrite another trainer's content.
	translationAuthHook := NewScenarioStepTranslationAuthorizationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(translationAuthHook); err != nil {
		log.Printf("Failed to register scenario step translation authorization hook: %v", err)
	} else {
		log.Println("Scenario step translation authorization hook registered")
	}

	scenarioTranslationAuthHook := NewScenarioTranslationAuthorizationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(scenarioTranslationAuthHook); err != nil {
		log.Printf("Failed to register scenario translation authorization hook: %v", err)
	} else {
		log.Println("Scenario translation authorization hook registered")
	}

	// Hook stamping which version of a step each translation was written
	// against. Registered before the question hook only for readability; the
	// registry orders by priority, not by registration order.
	translationStampHook := NewScenarioStepTranslationStampHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(translationStampHook); err != nil {
		log.Printf("Failed to register scenario step translation stamp hook: %v", err)
	} else {
		log.Println("Scenario step translation stamp hook registered")
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
