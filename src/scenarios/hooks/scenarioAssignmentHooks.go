package scenarioHooks

import (
	"fmt"
	"log/slog"

	"soli/formations/src/entityManagement/hooks"
	groupServices "soli/formations/src/groups/services"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ScenarioAssignmentAuthorizationHook verifies group/org authorization before
// creating, updating, or deleting an assignment.
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
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete, hooks.AfterDelete}
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
	case hooks.BeforeUpdate:
		return h.handleBeforeUpdate(ctx)
	case hooks.BeforeDelete:
		return h.handleBeforeDelete(ctx)
	case hooks.AfterDelete:
		return h.handleAfterDelete(ctx)
	}
	return nil
}

func (h *ScenarioAssignmentAuthorizationHook) handleBeforeCreate(ctx *hooks.HookContext) error {
	// Admin bypasses all authorization checks
	if ctx.IsAdmin() {
		if assignment, ok := ctx.NewEntity.(*models.ScenarioAssignment); ok && ctx.UserID != "" {
			assignment.CreatedByID = ctx.UserID
		}
		return nil
	}

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

	// Check org-level authorization when assigning to an organization
	if assignment.OrganizationID != nil && ctx.UserID != "" {
		canManage, err := h.canUserManageOrg(*assignment.OrganizationID, ctx.UserID)
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !canManage {
			return utils.PermissionDeniedError("manage scenario assignments for", "organization")
		}
	}

	return nil
}

func (h *ScenarioAssignmentAuthorizationHook) handleBeforeUpdate(ctx *hooks.HookContext) error {
	// Admin bypasses all authorization checks
	if ctx.IsAdmin() {
		return nil
	}

	// OldEntity contains the existing assignment loaded by the service
	assignment, ok := ctx.OldEntity.(*models.ScenarioAssignment)
	if !ok {
		return fmt.Errorf("expected *models.ScenarioAssignment in OldEntity, got %T", ctx.OldEntity)
	}

	// Check group-level authorization for group-scoped assignments
	if assignment.GroupID != nil && ctx.UserID != "" {
		canManage, err := h.groupService.CanUserManageGroup(*assignment.GroupID, ctx.UserID)
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !canManage {
			return utils.PermissionDeniedError("update scenario assignments for", "group")
		}
	}

	// Check org-level authorization for org-scoped assignments
	if assignment.OrganizationID != nil && ctx.UserID != "" {
		canManage, err := h.canUserManageOrg(*assignment.OrganizationID, ctx.UserID)
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !canManage {
			return utils.PermissionDeniedError("manage scenario assignments for", "organization")
		}
	}

	return nil
}

func (h *ScenarioAssignmentAuthorizationHook) handleBeforeDelete(ctx *hooks.HookContext) error {
	// Admin bypasses all authorization checks
	if ctx.IsAdmin() {
		return nil
	}

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

	// Check org-level authorization when deleting an org-scoped assignment
	if assignment.OrganizationID != nil && ctx.UserID != "" {
		canManage, err := h.canUserManageOrg(*assignment.OrganizationID, ctx.UserID)
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !canManage {
			return utils.PermissionDeniedError("manage scenario assignments for", "organization")
		}
	}

	return nil
}

func (h *ScenarioAssignmentAuthorizationHook) handleAfterDelete(ctx *hooks.HookContext) error {
	assignment, ok := ctx.NewEntity.(*models.ScenarioAssignment)
	if !ok {
		slog.Warn("AfterDelete: expected *models.ScenarioAssignment", "got", fmt.Sprintf("%T", ctx.NewEntity))
		return nil
	}

	if assignment.GroupID == nil {
		return nil
	}

	// Get active group member user IDs
	var memberUserIDs []string
	if err := h.db.Table("group_members").
		Where("group_id = ? AND is_active = ?", *assignment.GroupID, true).
		Pluck("user_id", &memberUserIDs).Error; err != nil {
		slog.Warn("AfterDelete: failed to load group members",
			"group_id", assignment.GroupID,
			"error", err)
		return nil
	}

	if len(memberUserIDs) == 0 {
		return nil
	}

	// Abandon all active/in_progress sessions for these users on this scenario
	result := h.db.Model(&models.ScenarioSession{}).
		Where("user_id IN ? AND scenario_id = ? AND status IN ?",
			memberUserIDs, assignment.ScenarioID, []string{"active", "in_progress"}).
		Updates(map[string]any{"status": "abandoned"})

	if result.Error != nil {
		slog.Warn("AfterDelete: failed to abandon sessions",
			"group_id", assignment.GroupID,
			"scenario_id", assignment.ScenarioID,
			"error", result.Error)
		return nil
	}

	if result.RowsAffected > 0 {
		slog.Info("AfterDelete: abandoned active sessions after assignment removal",
			"group_id", assignment.GroupID,
			"scenario_id", assignment.ScenarioID,
			"sessions_abandoned", result.RowsAffected)
	}

	return nil
}

// canUserManageOrg checks if a user is a manager or owner of the given organization.
func (h *ScenarioAssignmentAuthorizationHook) canUserManageOrg(orgID uuid.UUID, userID string) (bool, error) {
	var orgMember orgModels.OrganizationMember
	result := h.db.Where("organization_id = ? AND user_id = ? AND is_active = ?", orgID, userID, true).First(&orgMember)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, result.Error
	}
	return orgMember.IsManager(), nil
}
