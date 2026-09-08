package scenarioHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	groupServices "soli/formations/src/groups/services"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// ScenarioAuthorizationHook gates update/delete on Scenario itself.
//
// Layer 1 lets Members PATCH and DELETE scenarios; this hook ensures they
// can only do so on scenarios they actually manage (creator, org-manager
// of the scenario's org, or group-manager of any group it's assigned to).
//
// Reuses the CanManageScenario helper from scenarioStepHooks.go so the
// authorization rule for editing a scenario stays consistent with editing
// its steps and questions.
//
// CREATE is handled separately at the route level — platform-wide POST
// /scenarios is admin-only; org / group managers create via the dedicated
// /organizations/:id/scenarios and /groups/:groupId/scenarios endpoints.
type ScenarioAuthorizationHook struct {
	db           *gorm.DB
	groupService groupServices.GroupService
	hooks.BaseHook
}

// NewScenarioAuthorizationHook builds a new scenario authorization hook.
func NewScenarioAuthorizationHook(db *gorm.DB) hooks.Hook {
	return &ScenarioAuthorizationHook{
		db:           db,
		groupService: groupServices.NewGroupService(db),
		BaseHook: hooks.BaseHook{
			Name:       "scenario_authorization",
			EntityName: "Scenario",
			HookTypes:  []hooks.HookType{hooks.BeforeUpdate, hooks.BeforeDelete},
			Enabled:    true,
			Priority:   10,
		},
	}
}

func (h *ScenarioAuthorizationHook) Execute(ctx *hooks.HookContext) error {
	// Admin bypasses all checks.
	if ctx.IsAdmin() {
		return nil
	}

	switch ctx.HookType {
	case hooks.BeforeUpdate:
		return h.checkExisting(ctx, ctx.OldEntity, "update", "scenario")
	case hooks.BeforeDelete:
		return h.checkExisting(ctx, ctx.NewEntity, "delete", "scenario")
	}
	return nil
}

func (h *ScenarioAuthorizationHook) checkExisting(ctx *hooks.HookContext, raw any, action, entityLabel string) error {
	scenario, ok := raw.(*models.Scenario)
	if !ok {
		return fmt.Errorf("expected *models.Scenario, got %T", raw)
	}
	allowed, err := CanManageScenario(h.db, h.groupService, scenario, ctx.UserID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !allowed {
		return utils.PermissionDeniedError(action, entityLabel)
	}
	return nil
}
