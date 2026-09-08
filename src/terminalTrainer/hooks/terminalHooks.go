package terminalHooks

import (
	"fmt"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/entityManagement/hooks"
	paymentServices "soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// ========================
// Terminal Owner Permission Hook
// ========================

type TerminalOwnerPermissionHook struct {
	db *gorm.DB
	hooks.BaseHook
}

func NewTerminalOwnerPermissionHook(db *gorm.DB) hooks.Hook {
	return &TerminalOwnerPermissionHook{
		db: db,
		BaseHook: hooks.BaseHook{
			Name:       "terminal_owner_patch_permission",
			EntityName: "Terminal",
			HookTypes:  []hooks.HookType{hooks.AfterCreate},
			Enabled:    true,
			Priority:   100,
		},
	}
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
	db *gorm.DB
	hooks.BaseHook
}

func NewTerminalCleanupHook(db *gorm.DB) hooks.Hook {
	return &TerminalCleanupHook{
		db: db,
		BaseHook: hooks.BaseHook{
			Name:       "terminal_permission_cleanup",
			EntityName: "Terminal",
			HookTypes:  []hooks.HookType{hooks.AfterDelete},
			Enabled:    true,
			Priority:   100,
		},
	}
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

	// Register Terminal budget enforcement hook (MR-CORE-5 — BeforeCreate).
	// Wraps the request in a transaction that locks the user's (or org's)
	// active rows via SELECT FOR UPDATE so concurrent session starts can't
	// race past the budget cap. Also denormalises the size's CPU/RAM
	// footprint onto the Terminal row for race-safe future accounting.
	eps := paymentServices.NewEffectivePlanService(db)
	budgetHook := NewTerminalBudgetHook(db, eps, paymentServices.NewQuotaService(db, eps))
	if err := hooks.GlobalHookRegistry.RegisterHook(budgetHook); err != nil {
		utils.Error("❌ Failed to register Terminal budget hook: %v", err)
	} else {
		utils.Info("✅ Terminal budget hook registered")
	}

	// Re-provision a user's tt-backend key budget after the entity changes
	// that can move their ceiling: a personal subscription, an organization
	// role plan, an organization membership. One hook instance per entity,
	// since a hook declares a single entity name.
	terminalService := services.NewTerminalTrainerService(db)
	for _, entityName := range KeyBudgetSyncEntities {
		syncHook := NewTerminalKeyBudgetSyncHook(db, terminalService, entityName)
		if err := hooks.GlobalHookRegistry.RegisterHook(syncHook); err != nil {
			utils.Error("❌ Failed to register terminal key budget sync hook for %s: %v", entityName, err)
		} else {
			utils.Info("✅ Terminal key budget sync hook registered for %s", entityName)
		}
	}

	utils.Info("🔗 Terminal hooks initialization complete")
}
