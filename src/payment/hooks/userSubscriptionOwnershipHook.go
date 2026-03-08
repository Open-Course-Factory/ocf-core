package paymentHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/models"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// UserSubscriptionOwnershipHook enforces that only the owner (or admin) can
// create, update, or delete a UserSubscription.
type UserSubscriptionOwnershipHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewUserSubscriptionOwnershipHook(db *gorm.DB) hooks.Hook {
	return &UserSubscriptionOwnershipHook{
		db:       db,
		enabled:  true,
		priority: 10,
	}
}

func (h *UserSubscriptionOwnershipHook) GetName() string {
	return "user_subscription_ownership"
}

func (h *UserSubscriptionOwnershipHook) GetEntityName() string {
	return "UserSubscription"
}

func (h *UserSubscriptionOwnershipHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete}
}

func (h *UserSubscriptionOwnershipHook) IsEnabled() bool {
	return h.enabled
}

func (h *UserSubscriptionOwnershipHook) GetPriority() int {
	return h.priority
}

func (h *UserSubscriptionOwnershipHook) Execute(ctx *hooks.HookContext) error {
	switch ctx.HookType {
	case hooks.BeforeCreate:
		return h.handleBeforeCreate(ctx)
	case hooks.BeforeUpdate:
		return h.handleBeforeUpdate(ctx)
	case hooks.BeforeDelete:
		return h.handleBeforeDelete(ctx)
	}
	return nil
}

func (h *UserSubscriptionOwnershipHook) handleBeforeCreate(ctx *hooks.HookContext) error {
	sub, ok := ctx.NewEntity.(*models.UserSubscription)
	if !ok {
		return fmt.Errorf("expected *models.UserSubscription, got %T", ctx.NewEntity)
	}

	// Admin can create for any user
	if ctx.IsAdmin() {
		return nil
	}

	// Force UserID from authenticated user to prevent impersonation
	if ctx.UserID != "" {
		sub.UserID = ctx.UserID
	}

	return nil
}

func (h *UserSubscriptionOwnershipHook) handleBeforeUpdate(ctx *hooks.HookContext) error {
	// Admin bypasses ownership checks
	if ctx.IsAdmin() {
		return nil
	}

	// OldEntity contains the existing entity loaded by the service
	existingSub, ok := ctx.OldEntity.(*models.UserSubscription)
	if !ok {
		return fmt.Errorf("expected *models.UserSubscription in OldEntity, got %T", ctx.OldEntity)
	}

	// Verify ownership
	if existingSub.UserID != ctx.UserID {
		return utils.PermissionDeniedError("update", "user subscription")
	}

	return nil
}

func (h *UserSubscriptionOwnershipHook) handleBeforeDelete(ctx *hooks.HookContext) error {
	// Admin bypasses ownership checks
	if ctx.IsAdmin() {
		return nil
	}

	// NewEntity contains the existing entity loaded by the service before delete
	existingSub, ok := ctx.NewEntity.(*models.UserSubscription)
	if !ok {
		return fmt.Errorf("expected *models.UserSubscription in NewEntity, got %T", ctx.NewEntity)
	}

	// Verify ownership
	if existingSub.UserID != ctx.UserID {
		return utils.PermissionDeniedError("delete", "user subscription")
	}

	return nil
}
