package authHooks

import (
	"fmt"

	"soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// UserSettingsOwnershipHook enforces that only the owner (or admin) can update UserSettings.
// Members can only PATCH per Casbin, so only BeforeUpdate is needed.
type UserSettingsOwnershipHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewUserSettingsOwnershipHook(db *gorm.DB) hooks.Hook {
	return &UserSettingsOwnershipHook{
		db:       db,
		enabled:  true,
		priority: 10,
	}
}

func (h *UserSettingsOwnershipHook) GetName() string {
	return "user_settings_ownership"
}

func (h *UserSettingsOwnershipHook) GetEntityName() string {
	return "UserSettings"
}

func (h *UserSettingsOwnershipHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeUpdate}
}

func (h *UserSettingsOwnershipHook) IsEnabled() bool {
	return h.enabled
}

func (h *UserSettingsOwnershipHook) GetPriority() int {
	return h.priority
}

func (h *UserSettingsOwnershipHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType != hooks.BeforeUpdate {
		return nil
	}

	// Admin bypasses ownership checks
	if ctx.IsAdmin() {
		return nil
	}

	// OldEntity contains the existing entity loaded by the service before the hook
	existingSettings, ok := ctx.OldEntity.(*models.UserSettings)
	if !ok {
		return fmt.Errorf("expected *models.UserSettings in OldEntity, got %T", ctx.OldEntity)
	}

	// Verify ownership
	if existingSettings.UserID != ctx.UserID {
		return utils.PermissionDeniedError("update", "user settings")
	}

	return nil
}
