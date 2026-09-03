package scenarioHooks

import (
	"fmt"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	groupServices "soli/formations/src/groups/services"
	"soli/formations/src/scenarios/models"

	"gorm.io/gorm"
)

// ScenarioArchiveAuthorizationHook gates the framework's archive and
// unarchive actions on the same rule that already governs PATCH and DELETE:
// CanManageScenario. The action route is open to every member at Layer 2, so
// this hook is the only thing standing between a learner and a retired
// scenario. It refuses with the framework's unauthorized error so the route
// answers 403 rather than the 409 reserved for state refusals.
type ScenarioArchiveAuthorizationHook struct {
	db           *gorm.DB
	groupService groupServices.GroupService
}

func NewScenarioArchiveAuthorizationHook(db *gorm.DB) hooks.Hook {
	return &ScenarioArchiveAuthorizationHook{
		db:           db,
		groupService: groupServices.NewGroupService(db),
	}
}

func (h *ScenarioArchiveAuthorizationHook) GetName() string       { return "scenario_archive_authorization" }
func (h *ScenarioArchiveAuthorizationHook) GetEntityName() string { return "Scenario" }
func (h *ScenarioArchiveAuthorizationHook) IsEnabled() bool       { return true }
func (h *ScenarioArchiveAuthorizationHook) GetPriority() int      { return 10 }
func (h *ScenarioArchiveAuthorizationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeArchive, hooks.BeforeUnarchive}
}

func (h *ScenarioArchiveAuthorizationHook) Execute(ctx *hooks.HookContext) error {
	if ctx.IsAdmin() {
		return nil
	}
	scenario, ok := ctx.NewEntity.(*models.Scenario)
	if !ok {
		return fmt.Errorf("expected *models.Scenario, got %T", ctx.NewEntity)
	}
	allowed, err := CanManageScenario(h.db, h.groupService, scenario, ctx.UserID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !allowed {
		return entityErrors.NewUnauthorizedError(ctx.UserID, "scenario", string(ctx.HookType))
	}
	return nil
}
