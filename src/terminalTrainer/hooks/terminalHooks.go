package terminalHooks

import (
	"fmt"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/entityManagement/hooks"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// ========================
// Terminal Owner Permission Hook
// ========================

type TerminalOwnerPermissionHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewTerminalOwnerPermissionHook(db *gorm.DB) hooks.Hook {
	return &TerminalOwnerPermissionHook{
		db:       db,
		enabled:  true,
		priority: 100,
	}
}

func (h *TerminalOwnerPermissionHook) GetName() string {
	return "terminal_owner_patch_permission"
}

func (h *TerminalOwnerPermissionHook) GetEntityName() string {
	return "Terminal"
}

func (h *TerminalOwnerPermissionHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.AfterCreate}
}

func (h *TerminalOwnerPermissionHook) IsEnabled() bool {
	return h.enabled
}

func (h *TerminalOwnerPermissionHook) GetPriority() int {
	return h.priority
}

func (h *TerminalOwnerPermissionHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType != hooks.AfterCreate {
		return nil
	}

	terminal, ok := ctx.NewEntity.(*terminalModels.Terminal)
	if !ok {
		return fmt.Errorf("expected *terminalModels.Terminal, got %T", ctx.NewEntity)
	}

	route := fmt.Sprintf("/api/v1/terminals/%s", terminal.ID.String())
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	opts.WarnOnError = true

	err := utils.AddPolicy(casdoor.Enforcer, terminal.UserID, route, "PATCH", opts)
	if err != nil {
		return fmt.Errorf("failed to grant owner PATCH permission: %w", err)
	}

	utils.Info("✅ Granted PATCH permission to terminal owner %s for terminal %s", terminal.UserID, terminal.ID)
	return nil
}

// ========================
// Terminal Cleanup Hook
// ========================

type TerminalCleanupHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewTerminalCleanupHook(db *gorm.DB) hooks.Hook {
	return &TerminalCleanupHook{
		db:       db,
		enabled:  true,
		priority: 100,
	}
}

func (h *TerminalCleanupHook) GetName() string {
	return "terminal_permission_cleanup"
}

func (h *TerminalCleanupHook) GetEntityName() string {
	return "Terminal"
}

func (h *TerminalCleanupHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.AfterDelete}
}

func (h *TerminalCleanupHook) IsEnabled() bool {
	return h.enabled
}

func (h *TerminalCleanupHook) GetPriority() int {
	return h.priority
}

func (h *TerminalCleanupHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType != hooks.AfterDelete {
		return nil
	}

	terminal, ok := ctx.NewEntity.(*terminalModels.Terminal)
	if !ok {
		return fmt.Errorf("expected *terminalModels.Terminal, got %T", ctx.NewEntity)
	}

	route := fmt.Sprintf("/api/v1/terminals/%s", terminal.ID.String())
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	opts.WarnOnError = true

	err := utils.RemoveFilteredPolicy(casdoor.Enforcer, 1, opts, route)
	if err != nil {
		return fmt.Errorf("failed to remove terminal policies: %w", err)
	}

	utils.Info("✅ Removed all permissions for terminal %s", terminal.ID)
	return nil
}

// ========================
// Terminal Share Permission Hook
// ========================

type TerminalSharePermissionHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewTerminalSharePermissionHook(db *gorm.DB) hooks.Hook {
	return &TerminalSharePermissionHook{
		db:       db,
		enabled:  true,
		priority: 100,
	}
}

func (h *TerminalSharePermissionHook) GetName() string {
	return "terminal_share_patch_permission"
}

func (h *TerminalSharePermissionHook) GetEntityName() string {
	return "TerminalShare"
}

func (h *TerminalSharePermissionHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.AfterCreate}
}

func (h *TerminalSharePermissionHook) IsEnabled() bool {
	return h.enabled
}

func (h *TerminalSharePermissionHook) GetPriority() int {
	return h.priority
}

func (h *TerminalSharePermissionHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType != hooks.AfterCreate {
		return nil
	}

	share, ok := ctx.NewEntity.(*terminalModels.TerminalShare)
	if !ok {
		return fmt.Errorf("expected *terminalModels.TerminalShare, got %T", ctx.NewEntity)
	}

	// Only grant PATCH for "write" and "owner" access levels
	if share.AccessLevel != terminalModels.AccessLevelWrite && share.AccessLevel != terminalModels.AccessLevelOwner {
		utils.Debug("🔒 Not granting PATCH permission - access level '%s' doesn't allow editing", share.AccessLevel)
		return nil
	}

	// Only handle user shares for now
	if !share.IsUserShare() {
		utils.Debug("📋 Skipping group share - group member permissions handled separately")
		return nil
	}

	route := fmt.Sprintf("/api/v1/terminals/%s", share.TerminalID.String())
	recipientUserID := *share.SharedWithUserID

	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	opts.WarnOnError = true

	err := utils.AddPolicy(casdoor.Enforcer, recipientUserID, route, "PATCH", opts)
	if err != nil {
		return fmt.Errorf("failed to grant shared user PATCH permission: %w", err)
	}

	utils.Info("✅ Granted PATCH permission to shared user %s for terminal %s (access level: %s)",
		recipientUserID, share.TerminalID, share.AccessLevel)
	return nil
}

// ========================
// Terminal Share Revoke Hook
// ========================

type TerminalShareRevokeHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewTerminalShareRevokeHook(db *gorm.DB) hooks.Hook {
	return &TerminalShareRevokeHook{
		db:       db,
		enabled:  true,
		priority: 100,
	}
}

func (h *TerminalShareRevokeHook) GetName() string {
	return "terminal_share_revoke_permission"
}

func (h *TerminalShareRevokeHook) GetEntityName() string {
	return "TerminalShare"
}

func (h *TerminalShareRevokeHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.AfterDelete}
}

func (h *TerminalShareRevokeHook) IsEnabled() bool {
	return h.enabled
}

func (h *TerminalShareRevokeHook) GetPriority() int {
	return h.priority
}

func (h *TerminalShareRevokeHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType != hooks.AfterDelete {
		return nil
	}

	share, ok := ctx.NewEntity.(*terminalModels.TerminalShare)
	if !ok {
		return fmt.Errorf("expected *terminalModels.TerminalShare, got %T", ctx.NewEntity)
	}

	// Only handle user shares
	if !share.IsUserShare() {
		utils.Debug("📋 Skipping group share cleanup - handled separately")
		return nil
	}

	route := fmt.Sprintf("/api/v1/terminals/%s", share.TerminalID.String())
	recipientUserID := *share.SharedWithUserID

	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	opts.WarnOnError = true

	err := utils.RemovePolicy(casdoor.Enforcer, recipientUserID, route, "PATCH", opts)
	if err != nil {
		return fmt.Errorf("failed to remove shared user PATCH permission: %w", err)
	}

	utils.Info("✅ Removed PATCH permission from shared user %s for terminal %s",
		recipientUserID, share.TerminalID)
	return nil
}

// ========================
// Init Function
// ========================

func InitTerminalHooks(db *gorm.DB) {
	utils.Info("🔗 Initializing terminal hooks...")

	// Register Terminal owner permission hook
	ownerHook := NewTerminalOwnerPermissionHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(ownerHook); err != nil {
		utils.Error("❌ Failed to register Terminal owner permission hook: %v", err)
	} else {
		utils.Info("✅ Terminal owner permission hook registered")
	}

	// Register Terminal cleanup hook
	cleanupHook := NewTerminalCleanupHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(cleanupHook); err != nil {
		utils.Error("❌ Failed to register Terminal cleanup hook: %v", err)
	} else {
		utils.Info("✅ Terminal cleanup hook registered")
	}

	// Register TerminalShare permission hook
	shareHook := NewTerminalSharePermissionHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(shareHook); err != nil {
		utils.Error("❌ Failed to register TerminalShare permission hook: %v", err)
	} else {
		utils.Info("✅ TerminalShare permission hook registered")
	}

	// Register TerminalShare revoke hook
	revokeHook := NewTerminalShareRevokeHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(revokeHook); err != nil {
		utils.Error("❌ Failed to register TerminalShare revoke hook: %v", err)
	} else {
		utils.Info("✅ TerminalShare revoke hook registered")
	}

	utils.Info("🔗 Terminal hooks initialization complete")
}
