package terminalHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// ========================
// TerminalShare Ownership Hook (BeforeCreate)
// ========================

// TerminalShareOwnershipHook verifies the authenticated user owns the terminal
// being shared, and forces SharedByUserID from the authenticated user.
// Admins bypass ownership checks.
type TerminalShareOwnershipHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewTerminalShareOwnershipHook(db *gorm.DB) hooks.Hook {
	return &TerminalShareOwnershipHook{
		db:       db,
		enabled:  true,
		priority: 10,
	}
}

func (h *TerminalShareOwnershipHook) GetName() string {
	return "terminal_share_ownership"
}

func (h *TerminalShareOwnershipHook) GetEntityName() string {
	return "TerminalShare"
}

func (h *TerminalShareOwnershipHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate}
}

func (h *TerminalShareOwnershipHook) IsEnabled() bool {
	return h.enabled
}

func (h *TerminalShareOwnershipHook) GetPriority() int {
	return h.priority
}

func (h *TerminalShareOwnershipHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType != hooks.BeforeCreate {
		return nil
	}

	share, ok := ctx.NewEntity.(*terminalModels.TerminalShare)
	if !ok {
		return fmt.Errorf("expected *terminalModels.TerminalShare, got %T", ctx.NewEntity)
	}

	// Admin can share any terminal
	if ctx.IsAdmin() {
		return nil
	}

	// Set SharedByUserID from authenticated user
	if ctx.UserID != "" {
		share.SharedByUserID = ctx.UserID
	}

	// Verify authenticated user owns the terminal being shared
	if ctx.UserID != "" {
		var terminal terminalModels.Terminal
		err := h.db.Where("id = ?", share.TerminalID).First(&terminal).Error
		if err != nil {
			return fmt.Errorf("terminal not found: %w", err)
		}
		if terminal.UserID != ctx.UserID {
			return utils.PermissionDeniedError("share", "terminal")
		}
	}

	return nil
}
