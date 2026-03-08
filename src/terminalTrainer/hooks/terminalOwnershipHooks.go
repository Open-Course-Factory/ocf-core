package terminalHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// ========================
// Terminal Ownership Hook (BeforeCreate)
// ========================

// TerminalOwnershipHook forces the UserID from the authenticated user on create,
// preventing impersonation. Admins bypass this check.
type TerminalOwnershipHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewTerminalOwnershipHook(db *gorm.DB) hooks.Hook {
	return &TerminalOwnershipHook{
		db:       db,
		enabled:  true,
		priority: 10,
	}
}

func (h *TerminalOwnershipHook) GetName() string {
	return "terminal_ownership"
}

func (h *TerminalOwnershipHook) GetEntityName() string {
	return "Terminal"
}

func (h *TerminalOwnershipHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate}
}

func (h *TerminalOwnershipHook) IsEnabled() bool {
	return h.enabled
}

func (h *TerminalOwnershipHook) GetPriority() int {
	return h.priority
}

func (h *TerminalOwnershipHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType != hooks.BeforeCreate {
		return nil
	}

	terminal, ok := ctx.NewEntity.(*terminalModels.Terminal)
	if !ok {
		return fmt.Errorf("expected *terminalModels.Terminal, got %T", ctx.NewEntity)
	}

	// Admin can create for any user
	if ctx.IsAdmin() {
		return nil
	}

	// Force UserID from authenticated user to prevent impersonation
	if ctx.UserID != "" {
		terminal.UserID = ctx.UserID
	}

	return nil
}

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
