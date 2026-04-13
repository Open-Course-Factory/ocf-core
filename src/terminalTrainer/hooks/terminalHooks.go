package terminalHooks

import (
	"fmt"

	access "soli/formations/src/auth/access"
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
// Init Function
// ========================

func InitTerminalHooks(db *gorm.DB) {
	utils.Info("🔗 Initializing terminal hooks...")

	// Register Terminal ownership hook (BeforeCreate - prevents impersonation)
	if err := hooks.GlobalHookRegistry.RegisterHook(hooks.NewOwnershipHook(db, "Terminal", access.OwnershipConfig{
		OwnerField: "UserID", Operations: []string{"create"}, AdminBypass: true,
	})); err != nil {
		utils.Error("❌ Failed to register Terminal ownership hook: %v", err)
	} else {
		utils.Info("✅ Terminal ownership hook registered")
	}

	// Register Terminal owner permission hook (AfterCreate - grants PATCH)
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

	utils.Info("🔗 Terminal hooks initialization complete")
}
