package scenarioHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	groupServices "soli/formations/src/groups/services"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

// ScenarioAssignmentAuthorizationHook verifies group ownership before creating or deleting an assignment
type ScenarioAssignmentAuthorizationHook struct {
	db           *gorm.DB
	groupService groupServices.GroupService
	enabled      bool
	priority     int
}

func NewScenarioAssignmentAuthorizationHook(db *gorm.DB) hooks.Hook {
	return &ScenarioAssignmentAuthorizationHook{
		db:           db,
		groupService: groupServices.NewGroupService(db),
		enabled:      true,
		priority:     10,
	}
}

func (h *ScenarioAssignmentAuthorizationHook) GetName() string {
	return "scenario_assignment_authorization"
}

func (h *ScenarioAssignmentAuthorizationHook) GetEntityName() string {
	return "ScenarioAssignment"
}

func (h *ScenarioAssignmentAuthorizationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeDelete}
}

func (h *ScenarioAssignmentAuthorizationHook) IsEnabled() bool {
	return h.enabled
}

func (h *ScenarioAssignmentAuthorizationHook) GetPriority() int {
	return h.priority
}

func (h *ScenarioAssignmentAuthorizationHook) Execute(ctx *hooks.HookContext) error {
	switch ctx.HookType {
	case hooks.BeforeCreate:
		return h.handleBeforeCreate(ctx)
	case hooks.BeforeDelete:
		return h.handleBeforeDelete(ctx)
	}
	return nil
}

func (h *ScenarioAssignmentAuthorizationHook) handleBeforeCreate(ctx *hooks.HookContext) error {
	assignment, ok := ctx.NewEntity.(*models.ScenarioAssignment)
	if !ok {
		return fmt.Errorf("expected *models.ScenarioAssignment, got %T", ctx.NewEntity)
	}

	// Set CreatedByID from the authenticated user
	if ctx.UserID != "" {
		assignment.CreatedByID = ctx.UserID
	}

	// Check group-level authorization when assigning to a group
	if assignment.GroupID != nil && ctx.UserID != "" {
		canManage, err := h.groupService.CanUserManageGroup(*assignment.GroupID, ctx.UserID)
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !canManage {
			return utils.PermissionDeniedError("assign scenarios to", "group")
		}
	}

	return nil
}

func (h *ScenarioAssignmentAuthorizationHook) handleBeforeDelete(ctx *hooks.HookContext) error {
	assignment, ok := ctx.NewEntity.(*models.ScenarioAssignment)
	if !ok {
		return fmt.Errorf("expected *models.ScenarioAssignment, got %T", ctx.NewEntity)
	}

	// Check group-level authorization when deleting a group assignment
	if assignment.GroupID != nil && ctx.UserID != "" {
		canManage, err := h.groupService.CanUserManageGroup(*assignment.GroupID, ctx.UserID)
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !canManage {
			return utils.PermissionDeniedError("remove scenario assignments from", "group")
		}
	}

	return nil
}
