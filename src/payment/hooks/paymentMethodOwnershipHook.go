package paymentHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/models"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// PaymentMethodOwnershipHook enforces that only the owner (or admin) can
// create, update, or delete a PaymentMethod.
type PaymentMethodOwnershipHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewPaymentMethodOwnershipHook(db *gorm.DB) hooks.Hook {
	return &PaymentMethodOwnershipHook{
		db:       db,
		enabled:  true,
		priority: 10,
	}
}

func (h *PaymentMethodOwnershipHook) GetName() string {
	return "payment_method_ownership"
}

func (h *PaymentMethodOwnershipHook) GetEntityName() string {
	return "PaymentMethod"
}

func (h *PaymentMethodOwnershipHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete}
}

func (h *PaymentMethodOwnershipHook) IsEnabled() bool {
	return h.enabled
}

func (h *PaymentMethodOwnershipHook) GetPriority() int {
	return h.priority
}

func (h *PaymentMethodOwnershipHook) Execute(ctx *hooks.HookContext) error {
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

func (h *PaymentMethodOwnershipHook) handleBeforeCreate(ctx *hooks.HookContext) error {
	pm, ok := ctx.NewEntity.(*models.PaymentMethod)
	if !ok {
		return fmt.Errorf("expected *models.PaymentMethod, got %T", ctx.NewEntity)
	}

	// Admin can create for any user
	if ctx.IsAdmin() {
		return nil
	}

	// Force UserID from authenticated user to prevent impersonation
	if ctx.UserID != "" {
		pm.UserID = ctx.UserID
	}

	return nil
}

func (h *PaymentMethodOwnershipHook) handleBeforeUpdate(ctx *hooks.HookContext) error {
	// Admin bypasses ownership checks
	if ctx.IsAdmin() {
		return nil
	}

	// OldEntity contains the existing entity loaded by the service
	existingPM, ok := ctx.OldEntity.(*models.PaymentMethod)
	if !ok {
		return fmt.Errorf("expected *models.PaymentMethod in OldEntity, got %T", ctx.OldEntity)
	}

	// Verify ownership
	if existingPM.UserID != ctx.UserID {
		return utils.PermissionDeniedError("update", "payment method")
	}

	return nil
}

func (h *PaymentMethodOwnershipHook) handleBeforeDelete(ctx *hooks.HookContext) error {
	// Admin bypasses ownership checks
	if ctx.IsAdmin() {
		return nil
	}

	// NewEntity contains the existing entity loaded by the service before delete
	existingPM, ok := ctx.NewEntity.(*models.PaymentMethod)
	if !ok {
		return fmt.Errorf("expected *models.PaymentMethod in NewEntity, got %T", ctx.NewEntity)
	}

	// Verify ownership
	if existingPM.UserID != ctx.UserID {
		return utils.PermissionDeniedError("delete", "payment method")
	}

	return nil
}
