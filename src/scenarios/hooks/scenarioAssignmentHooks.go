package scenarioHooks

import (
	"fmt"
	"log/slog"

	"soli/formations/src/entityManagement/hooks"
	groupModels "soli/formations/src/groups/models"
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
	hooks.BaseHook
}

func NewScenarioAssignmentAuthorizationHook(db *gorm.DB) hooks.Hook {
	return &ScenarioAssignmentAuthorizationHook{
		db:           db,
		groupService: groupServices.NewGroupService(db),
		BaseHook: hooks.BaseHook{
			Name:       "scenario_assignment_authorization",
			EntityName: "ScenarioAssignment",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete, hooks.AfterDelete},
			Enabled:    true,
			Priority:   10,
		},
	}
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
		assignment, ok := ctx.NewEntity.(*models.ScenarioAssignment)
		if !ok {
			return nil
		}
		if ctx.UserID != "" {
			assignment.CreatedByID = ctx.UserID
		}
		scenario, err := loadScenarioByID(h.db, assignment.ScenarioID)
		if err != nil {
			return err
		}
		return refuseCrossOrgAssignment(h.db, scenario, assignment)
	}

	assignment, ok := ctx.NewEntity.(*models.ScenarioAssignment)
	if !ok {
		return fmt.Errorf("expected *models.ScenarioAssignment, got %T", ctx.NewEntity)
	}

	// Set CreatedByID from the authenticated user
	if ctx.UserID != "" {
		assignment.CreatedByID = ctx.UserID
	}

	if err := h.deny(assignment, ctx.UserID, "assign scenarios to"); err != nil {
		return err
	}
	return h.refuseInvisibleScenario(assignment, ctx.UserID)
}

// refuseInvisibleScenario keeps an assignment inside what the caller may see
// (CanSeeScenario) and inside the scenario's organisation: a scenario id is
// not a permission, and an org scenario never reaches another org's class.
func (h *ScenarioAssignmentAuthorizationHook) refuseInvisibleScenario(assignment *models.ScenarioAssignment, userID string) error {
	scenario, err := loadScenarioByID(h.db, assignment.ScenarioID)
	if err != nil {
		return err
	}
	visible, err := CanSeeScenario(h.db, h.groupService, scenario, userID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !visible {
		return utils.PermissionDeniedError("assign", "scenario")
	}
	return refuseCrossOrgAssignment(h.db, scenario, assignment)
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

	return h.deny(assignment, ctx.UserID, "update scenario assignments for")
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

	return h.deny(assignment, ctx.UserID, "remove scenario assignments from")
}

// deny turns CanManageAssignment into the hook's verdict for one action.
func (h *ScenarioAssignmentAuthorizationHook) deny(assignment *models.ScenarioAssignment, userID, action string) error {
	allowed, err := CanManageAssignment(h.db, h.groupService, assignment, userID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !allowed {
		return utils.PermissionDeniedError(action, "group or organization")
	}
	return nil
}

// CanManageAssignment is the one owner of "may this user touch this
// assignment": manager of the assigned group, and of the assigned
// organization when the assignment names one. Shared by the write hooks and
// by the GET /scenario-assignments scope, so they cannot drift.
func CanManageAssignment(db *gorm.DB, groupSvc groupServices.GroupService, a *models.ScenarioAssignment, userID string) (bool, error) {
	if userID == "" || (a.GroupID == nil && a.OrganizationID == nil) {
		return false, nil
	}
	if a.GroupID != nil {
		ok, err := groupSvc.CanUserManageGroup(*a.GroupID, userID)
		if err != nil || !ok {
			return false, err
		}
	}
	if a.OrganizationID != nil {
		ok, err := CanUserManageOrg(db, *a.OrganizationID, userID)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

// ListableAssignmentIDs scopes GET /scenario-assignments: the caller manages
// the assignment's group / organization, or the assigned scenario itself
// (a scenario's author sees where it is deployed). Same per-row cost profile
// as ListableScenarioIDs; the scenario verdict is memoised per scenario.
func ListableAssignmentIDs(db *gorm.DB, groupSvc groupServices.GroupService, userID string) ([]string, error) {
	var assignments []models.ScenarioAssignment
	if err := db.Select("id", "scenario_id", "group_id", "organization_id").Find(&assignments).Error; err != nil {
		return nil, fmt.Errorf("load assignments for scoping: %w", err)
	}
	scenarioVerdicts := map[uuid.UUID]bool{}
	ids := make([]string, 0, len(assignments))
	for i := range assignments {
		a := &assignments[i]
		ok, err := CanManageAssignment(db, groupSvc, a, userID)
		if err != nil {
			return nil, err
		}
		if !ok {
			if ok, err = canManageScenarioByID(db, groupSvc, a.ScenarioID, userID, scenarioVerdicts); err != nil {
				return nil, err
			}
		}
		if ok {
			ids = append(ids, a.ID.String())
		}
	}
	return ids, nil
}

func canManageScenarioByID(db *gorm.DB, groupSvc groupServices.GroupService, scenarioID uuid.UUID, userID string, memo map[uuid.UUID]bool) (bool, error) {
	if verdict, seen := memo[scenarioID]; seen {
		return verdict, nil
	}
	var scenario models.Scenario
	err := db.Select("id", "created_by_id", "organization_id").Where("id = ?", scenarioID).First(&scenario).Error
	if err == gorm.ErrRecordNotFound {
		memo[scenarioID] = false
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load scenario %s: %w", scenarioID, err)
	}
	verdict, err := CanManageScenario(db, groupSvc, &scenario, userID)
	if err != nil {
		return false, err
	}
	memo[scenarioID] = verdict
	return verdict, nil
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

	// Abandon every session these users could still resume on this scenario.
	// The status list is models.OpenSessionStatuses; leaving any of
	// them behind hands the learner a session for a scenario they are no
	// longer assigned to, and for 'provisioning' the unique partial index
	// then blocks a fresh start as well.
	result := h.db.Model(&models.ScenarioSession{}).
		Where("user_id IN ? AND scenario_id = ? AND status IN ?",
			memberUserIDs, assignment.ScenarioID, models.OpenSessionStatuses).
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

// CanUserManageOrg reports whether the user is an active manager or owner of
// the organization: the one owner of that predicate for scenario management.
func CanUserManageOrg(db *gorm.DB, orgID uuid.UUID, userID string) (bool, error) {
	var orgMember orgModels.OrganizationMember
	err := db.Where("organization_id = ? AND user_id = ? AND is_active = ?", orgID, userID, true).First(&orgMember).Error
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return orgMember.IsManager(), nil
}

// refuseCrossOrgAssignment: an org scenario is assignable only to that org's
// classes (or to the org itself). Platform scenarios go anywhere. Applies to
// admins too — there is no legitimate cross-org assignment.
func refuseCrossOrgAssignment(db *gorm.DB, scenario *models.Scenario, assignment *models.ScenarioAssignment) error {
	if scenario.OrganizationID == nil {
		return nil
	}
	targetOrg := assignment.OrganizationID
	if assignment.GroupID != nil {
		var group groupModels.ClassGroup
		if err := db.Select("organization_id").First(&group, "id = ?", *assignment.GroupID).Error; err != nil {
			return fmt.Errorf("load group: %w", err)
		}
		targetOrg = group.OrganizationID
	}
	if targetOrg == nil || *targetOrg != *scenario.OrganizationID {
		return utils.PermissionDeniedError("assign outside its organisation", "scenario")
	}
	return nil
}
