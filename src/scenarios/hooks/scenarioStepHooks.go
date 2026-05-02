package scenarioHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	groupServices "soli/formations/src/groups/services"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// canManageScenario centralises the "can this user edit this scenario's
// content (steps + questions)?" rule. The user is allowed if any of the
// following holds:
//
//   - they are the scenario creator (CreatedByID),
//   - they are a manager or owner of the scenario's organization (when
//     the scenario is org-scoped),
//   - they are a manager or owner of any group the scenario is assigned
//     to (so group-scoped trainers can author their own labs).
//
// Platform admin bypass is handled by callers via ctx.IsAdmin().
func canManageScenario(db *gorm.DB, groupSvc groupServices.GroupService, scenario *models.Scenario, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	// Creator can always manage.
	if scenario.CreatedByID == userID {
		return true, nil
	}

	// Org manager / owner can manage scenarios of their org.
	if scenario.OrganizationID != nil {
		var orgMember orgModels.OrganizationMember
		err := db.Where("organization_id = ? AND user_id = ? AND is_active = ?",
			*scenario.OrganizationID, userID, true).First(&orgMember).Error
		if err == nil && orgMember.IsManager() {
			return true, nil
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return false, fmt.Errorf("load org member: %w", err)
		}
	}

	// Group manager of any group the scenario is assigned to.
	var groupIDs []uuid.UUID
	if err := db.Table("scenario_assignments").
		Where("scenario_id = ? AND scope = ? AND group_id IS NOT NULL", scenario.ID, "group").
		Pluck("group_id", &groupIDs).Error; err != nil {
		return false, fmt.Errorf("load scenario group assignments: %w", err)
	}
	for _, gid := range groupIDs {
		canManage, err := groupSvc.CanUserManageGroup(gid, userID)
		if err != nil {
			return false, fmt.Errorf("check group manage permission: %w", err)
		}
		if canManage {
			return true, nil
		}
	}

	return false, nil
}

// loadScenarioByID fetches a scenario by its ID. Returns a friendly error
// if the scenario does not exist.
func loadScenarioByID(db *gorm.DB, scenarioID uuid.UUID) (*models.Scenario, error) {
	var scenario models.Scenario
	if err := db.Where("id = ?", scenarioID).First(&scenario).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("scenario %s not found", scenarioID)
		}
		return nil, fmt.Errorf("load scenario: %w", err)
	}
	return &scenario, nil
}

// loadStepByID fetches a scenario step by its ID.
func loadStepByID(db *gorm.DB, stepID uuid.UUID) (*models.ScenarioStep, error) {
	var step models.ScenarioStep
	if err := db.Where("id = ?", stepID).First(&step).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("scenario step %s not found", stepID)
		}
		return nil, fmt.Errorf("load step: %w", err)
	}
	return &step, nil
}

// =============================================================================
// ScenarioStepAuthorizationHook
// =============================================================================

// ScenarioStepAuthorizationHook gates create/update/delete on ScenarioStep
// to users who can manage the parent scenario.
type ScenarioStepAuthorizationHook struct {
	db           *gorm.DB
	groupService groupServices.GroupService
	enabled      bool
	priority     int
}

// NewScenarioStepAuthorizationHook builds a new step authorization hook.
func NewScenarioStepAuthorizationHook(db *gorm.DB) hooks.Hook {
	return &ScenarioStepAuthorizationHook{
		db:           db,
		groupService: groupServices.NewGroupService(db),
		enabled:      true,
		priority:     10,
	}
}

func (h *ScenarioStepAuthorizationHook) GetName() string         { return "scenario_step_authorization" }
func (h *ScenarioStepAuthorizationHook) GetEntityName() string   { return "ScenarioStep" }
func (h *ScenarioStepAuthorizationHook) IsEnabled() bool         { return h.enabled }
func (h *ScenarioStepAuthorizationHook) GetPriority() int        { return h.priority }
func (h *ScenarioStepAuthorizationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete}
}

func (h *ScenarioStepAuthorizationHook) Execute(ctx *hooks.HookContext) error {
	// Admin bypasses all checks.
	if ctx.IsAdmin() {
		return nil
	}

	switch ctx.HookType {
	case hooks.BeforeCreate:
		return h.checkCreate(ctx)
	case hooks.BeforeUpdate:
		return h.checkUpdateOrDelete(ctx, ctx.OldEntity, "update", "scenario step")
	case hooks.BeforeDelete:
		return h.checkUpdateOrDelete(ctx, ctx.NewEntity, "delete", "scenario step")
	}
	return nil
}

func (h *ScenarioStepAuthorizationHook) checkCreate(ctx *hooks.HookContext) error {
	step, ok := ctx.NewEntity.(*models.ScenarioStep)
	if !ok {
		return fmt.Errorf("expected *models.ScenarioStep in NewEntity, got %T", ctx.NewEntity)
	}
	if step.ScenarioID == uuid.Nil {
		return fmt.Errorf("scenario_id is required to create a step")
	}
	scenario, err := loadScenarioByID(h.db, step.ScenarioID)
	if err != nil {
		return err
	}
	allowed, err := canManageScenario(h.db, h.groupService, scenario, ctx.UserID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !allowed {
		return utils.PermissionDeniedError("add steps to", "scenario")
	}
	return nil
}

func (h *ScenarioStepAuthorizationHook) checkUpdateOrDelete(ctx *hooks.HookContext, raw any, action, entityLabel string) error {
	step, ok := raw.(*models.ScenarioStep)
	if !ok {
		return fmt.Errorf("expected *models.ScenarioStep, got %T", raw)
	}
	scenario, err := loadScenarioByID(h.db, step.ScenarioID)
	if err != nil {
		return err
	}
	allowed, err := canManageScenario(h.db, h.groupService, scenario, ctx.UserID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !allowed {
		return utils.PermissionDeniedError(action, entityLabel)
	}
	return nil
}

// =============================================================================
// ScenarioStepQuestionAuthorizationHook
// =============================================================================

// ScenarioStepQuestionAuthorizationHook gates create/update/delete on
// ScenarioStepQuestion. Authorization is transitive: load the question's
// step, then the step's parent scenario, then run the standard scenario
// management check.
type ScenarioStepQuestionAuthorizationHook struct {
	db           *gorm.DB
	groupService groupServices.GroupService
	enabled      bool
	priority     int
}

// NewScenarioStepQuestionAuthorizationHook builds a new question authorization hook.
func NewScenarioStepQuestionAuthorizationHook(db *gorm.DB) hooks.Hook {
	return &ScenarioStepQuestionAuthorizationHook{
		db:           db,
		groupService: groupServices.NewGroupService(db),
		enabled:      true,
		priority:     10,
	}
}

func (h *ScenarioStepQuestionAuthorizationHook) GetName() string {
	return "scenario_step_question_authorization"
}
func (h *ScenarioStepQuestionAuthorizationHook) GetEntityName() string {
	return "ScenarioStepQuestion"
}
func (h *ScenarioStepQuestionAuthorizationHook) IsEnabled() bool { return h.enabled }
func (h *ScenarioStepQuestionAuthorizationHook) GetPriority() int { return h.priority }
func (h *ScenarioStepQuestionAuthorizationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete}
}

func (h *ScenarioStepQuestionAuthorizationHook) Execute(ctx *hooks.HookContext) error {
	if ctx.IsAdmin() {
		return nil
	}

	switch ctx.HookType {
	case hooks.BeforeCreate:
		return h.checkCreate(ctx)
	case hooks.BeforeUpdate:
		return h.checkExisting(ctx, ctx.OldEntity, "update", "scenario step question")
	case hooks.BeforeDelete:
		return h.checkExisting(ctx, ctx.NewEntity, "delete", "scenario step question")
	}
	return nil
}

func (h *ScenarioStepQuestionAuthorizationHook) checkCreate(ctx *hooks.HookContext) error {
	question, ok := ctx.NewEntity.(*models.ScenarioStepQuestion)
	if !ok {
		return fmt.Errorf("expected *models.ScenarioStepQuestion in NewEntity, got %T", ctx.NewEntity)
	}
	if question.StepID == uuid.Nil {
		return fmt.Errorf("step_id is required to create a question")
	}
	step, err := loadStepByID(h.db, question.StepID)
	if err != nil {
		return err
	}
	scenario, err := loadScenarioByID(h.db, step.ScenarioID)
	if err != nil {
		return err
	}
	allowed, err := canManageScenario(h.db, h.groupService, scenario, ctx.UserID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !allowed {
		return utils.PermissionDeniedError("add questions to", "scenario step")
	}
	return nil
}

func (h *ScenarioStepQuestionAuthorizationHook) checkExisting(ctx *hooks.HookContext, raw any, action, entityLabel string) error {
	question, ok := raw.(*models.ScenarioStepQuestion)
	if !ok {
		return fmt.Errorf("expected *models.ScenarioStepQuestion, got %T", raw)
	}
	step, err := loadStepByID(h.db, question.StepID)
	if err != nil {
		return err
	}
	scenario, err := loadScenarioByID(h.db, step.ScenarioID)
	if err != nil {
		return err
	}
	allowed, err := canManageScenario(h.db, h.groupService, scenario, ctx.UserID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !allowed {
		return utils.PermissionDeniedError(action, entityLabel)
	}
	return nil
}
