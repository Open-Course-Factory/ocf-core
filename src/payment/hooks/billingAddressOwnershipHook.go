package paymentHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/models"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// BillingAddressOwnershipHook enforces that only the owner (or admin) can
// create, update, or delete a BillingAddress.
type BillingAddressOwnershipHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewBillingAddressOwnershipHook(db *gorm.DB) hooks.Hook {
	return &BillingAddressOwnershipHook{
		db:       db,
		enabled:  true,
		priority: 10,
	}
}

func (h *BillingAddressOwnershipHook) GetName() string {
	return "billing_address_ownership"
}

func (h *BillingAddressOwnershipHook) GetEntityName() string {
	return "BillingAddress"
}

func (h *BillingAddressOwnershipHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete}
}

func (h *BillingAddressOwnershipHook) IsEnabled() bool {
	return h.enabled
}

func (h *BillingAddressOwnershipHook) GetPriority() int {
	return h.priority
}

func (h *BillingAddressOwnershipHook) Execute(ctx *hooks.HookContext) error {
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

func (h *BillingAddressOwnershipHook) handleBeforeCreate(ctx *hooks.HookContext) error {
	address, ok := ctx.NewEntity.(*models.BillingAddress)
	if !ok {
		return fmt.Errorf("expected *models.BillingAddress, got %T", ctx.NewEntity)
	}

	// Admin can create for any user
	if ctx.IsAdmin() {
		return nil
	}

	// Force UserID from authenticated user to prevent impersonation
	if ctx.UserID != "" {
		address.UserID = ctx.UserID
	}

	return nil
}

func (h *BillingAddressOwnershipHook) handleBeforeUpdate(ctx *hooks.HookContext) error {
	// Admin bypasses ownership checks
	if ctx.IsAdmin() {
		return nil
	}

	// OldEntity contains the existing entity loaded by the service
	existingAddress, ok := ctx.OldEntity.(*models.BillingAddress)
	if !ok {
		return fmt.Errorf("expected *models.BillingAddress in OldEntity, got %T", ctx.OldEntity)
	}

	// Verify ownership
	if existingAddress.UserID != ctx.UserID {
		return utils.PermissionDeniedError("update", "billing address")
	}

	return nil
}

func (h *BillingAddressOwnershipHook) handleBeforeDelete(ctx *hooks.HookContext) error {
	// Admin bypasses ownership checks
	if ctx.IsAdmin() {
		return nil
	}

	// NewEntity contains the existing entity loaded by the service before delete
	existingAddress, ok := ctx.NewEntity.(*models.BillingAddress)
	if !ok {
		return fmt.Errorf("expected *models.BillingAddress in NewEntity, got %T", ctx.NewEntity)
	}

	// Verify ownership
	if existingAddress.UserID != ctx.UserID {
		return utils.PermissionDeniedError("delete", "billing address")
	}

	return nil
}
