package scenarioHooks

import (
	"fmt"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	groupServices "soli/formations/src/groups/services"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
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
			HookTypes:  []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete},
			Enabled:    true,
			Priority:   10,
		},
	}
}

func (h *ScenarioAuthorizationHook) Execute(ctx *hooks.HookContext) error {
	// The public flag is a platform notion (models.PublicCatalogue): on an org
	// scenario it would promise a visibility the readers never grant, so it is
	// refused for everyone, admins included.
	if err := refusePublicOrgScenario(ctx); err != nil {
		return err
	}
	// Admin bypasses the authorization checks.
	if ctx.IsAdmin() {
		return nil
	}

	switch ctx.HookType {
	case hooks.BeforeUpdate:
		if err := refuseOrgChange(ctx); err != nil {
			return err
		}
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

// refuseOrgChange keeps a scenario in its organisation (#520): moving it, or
// turning it into a platform scenario with the nil UUID, is an administrator's
// act. Sending the stored organisation back unchanged is a normal edit.
func refuseOrgChange(ctx *hooks.HookContext) error {
	patch, _ := ctx.NewEntity.(map[string]any)
	orgID, patched := patch["organization_id"].(uuid.UUID)
	if !patched {
		return nil
	}
	if old, ok := ctx.OldEntity.(*models.Scenario); ok && old.OrganizationID != nil && *old.OrganizationID == orgID {
		return nil
	}
	return utils.PermissionDeniedError("move", "scenario")
}

var errPublicOrgScenario = entityErrors.NewValidationError("is_public", "an organisation's scenario cannot be public: only platform scenarios are")

func refusePublicOrgScenario(ctx *hooks.HookContext) error {
	switch ctx.HookType {
	case hooks.BeforeCreate:
		if s, ok := ctx.NewEntity.(*models.Scenario); ok && s.IsPublic && s.OrganizationID != nil {
			return errPublicOrgScenario
		}
	case hooks.BeforeUpdate:
		patch, ok := ctx.NewEntity.(map[string]any)
		if !ok {
			return nil
		}
		public, _ := patch["is_public"].(bool)
		if !public {
			return nil
		}
		if orgID, patched := patch["organization_id"].(uuid.UUID); patched {
			if orgID != uuid.Nil {
				return errPublicOrgScenario
			}
			return nil
		}
		if old, ok := ctx.OldEntity.(*models.Scenario); ok && old.OrganizationID != nil {
			return errPublicOrgScenario
		}
	}
	return nil
}
