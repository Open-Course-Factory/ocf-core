package services

import (
	"soli/formations/src/entityManagement/hooks"
	appUtils "soli/formations/src/utils"

	"gorm.io/gorm"
)

// RegisterOwnershipHooks wires the write-side ownership hooks for every entity
// that declared an OwnershipConfig at registration time. It is the single pass
// that turns the declarative OwnershipConfig (OwnerField + Operations) into the
// BeforeCreate/BeforeUpdate/BeforeDelete hooks that force the owner on create and
// verify ownership on update/delete.
//
// A read-only config (Operations: ["read"]) yields no write hook — request-time
// read scoping in the generic GET handlers covers that case separately.
//
// Called once at startup from main.go, AFTER all entities are registered (so
// their configs are stored in GlobalEntityRegistrationService) and AFTER the DB
// is ready (the update/delete hooks load the persisted owner through a *gorm.DB).
func RegisterOwnershipHooks(db *gorm.DB) {
	appUtils.Info("🔗 Initializing ownership hooks from registered OwnershipConfigs...")

	for entityName, config := range GlobalEntityRegistrationService.GetAllOwnershipConfigs() {
		hook := hooks.NewOwnershipHook(db, entityName, *config)
		if len(hook.GetHookTypes()) == 0 {
			// Read-only ownership config: no write op to guard, skip.
			continue
		}
		if err := hooks.GlobalHookRegistry.RegisterHook(hook); err != nil {
			appUtils.Error("❌ Failed to register %s ownership hook: %v", entityName, err)
		} else {
			appUtils.Info("✅ %s ownership hook registered", entityName)
		}
	}

	appUtils.Info("🔗 Ownership hooks initialization complete")
}
