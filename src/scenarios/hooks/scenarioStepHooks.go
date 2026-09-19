package scenarioHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	groupModels "soli/formations/src/groups/models"
	groupServices "soli/formations/src/groups/services"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CanManageScenario is the one owner of "may this user edit this scenario"
// (the scenario itself, its steps, questions, translations, lexicon). The
// user is allowed if any of the following holds:
//
//   - they are the scenario creator (CreatedByID),
//   - they are a manager or owner of the scenario's organisation,
//   - they manage a class of the scenario's organisation (a teacher works
//     on every lab of their school, not only the ones assigned to them).
//
// A scenario never leaves its organisation: being assigned to a class in
// another organisation grants nothing, and a platform scenario (no org) is
// managed by its creator and platform admins only. Admin bypass is handled
// by callers via ctx.IsAdmin() (hooks) or access.IsAdmin(roles) (controllers).
func CanManageScenario(db *gorm.DB, groupSvc groupServices.GroupService, scenario *models.Scenario, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	if scenario.CreatedByID == userID {
		return true, nil
	}
	if scenario.OrganizationID == nil {
		return false, nil
	}

	canManage, err := CanUserManageOrg(db, *scenario.OrganizationID, userID)
	if err != nil {
		return false, fmt.Errorf("load org member: %w", err)
	}
	if canManage {
		return true, nil
	}

	var managedClasses int64
	if err := db.Model(&groupModels.ClassGroup{}).
		Scopes(groupModels.ManagedByScope(userID)).
		Where("class_groups.organization_id = ?", *scenario.OrganizationID).
		Count(&managedClasses).Error; err != nil {
		return false, fmt.Errorf("count managed classes in org: %w", err)
	}
	return managedClasses > 0, nil
}

// CanSeeScenario is what assigning and copying require: the scenario is
// manageable by the user, or it is in the public catalogue. Read-side
// listing (ListableScenarioIDs) is the same rule applied to the whole table.
func CanSeeScenario(db *gorm.DB, groupSvc groupServices.GroupService, scenario *models.Scenario, userID string) (bool, error) {
	if scenario.InPublicCatalogue() {
		return true, nil
	}
	return CanManageScenario(db, groupSvc, scenario, userID)
}

// ListableScenarioIDs is the list-side twin of CanManageScenario: every
// scenario the caller may manage, plus the public ones (already visible in the
// learner catalogue). GET /scenarios and the write hooks share one predicate
// so they can never disagree on who sees what (issue #294).
//
// ponytail: one CanManageScenario call per non-public scenario, a handful of
// queries each. Fine at hundreds of scenarios; resolve the caller's managed
// org/group ids once and match in memory if the table grows past that.
func ListableScenarioIDs(db *gorm.DB, groupSvc groupServices.GroupService, userID string) ([]string, error) {
	var scenarios []models.Scenario
	if err := db.Select("id", "created_by_id", "organization_id", "is_public", "archived_at").Find(&scenarios).Error; err != nil {
		return nil, fmt.Errorf("load scenarios for scoping: %w", err)
	}
	ids := make([]string, 0, len(scenarios))
	for i := range scenarios {
		listable, err := CanSeeScenario(db, groupSvc, &scenarios[i], userID)
		if err != nil {
			return nil, err
		}
		if listable {
			ids = append(ids, scenarios[i].ID.String())
		}
	}
	return ids, nil
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
	hooks.BaseHook
}

// NewScenarioStepAuthorizationHook builds a new step authorization hook.
func NewScenarioStepAuthorizationHook(db *gorm.DB) hooks.Hook {
	return &ScenarioStepAuthorizationHook{
		db:           db,
		groupService: groupServices.NewGroupService(db),
		BaseHook: hooks.BaseHook{
			Name:       "scenario_step_authorization",
			EntityName: "ScenarioStep",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete},
			Enabled:    true,
			Priority:   10,
		},
	}
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
	allowed, err := CanManageScenario(h.db, h.groupService, scenario, ctx.UserID)
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
	allowed, err := CanManageScenario(h.db, h.groupService, scenario, ctx.UserID)
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
	hooks.BaseHook
}

// NewScenarioStepQuestionAuthorizationHook builds a new question authorization hook.
func NewScenarioStepQuestionAuthorizationHook(db *gorm.DB) hooks.Hook {
	return &ScenarioStepQuestionAuthorizationHook{
		db:           db,
		groupService: groupServices.NewGroupService(db),
		BaseHook: hooks.BaseHook{
			Name:       "scenario_step_question_authorization",
			EntityName: "ScenarioStepQuestion",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete},
			Enabled:    true,
			Priority:   10,
		},
	}
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
	allowed, err := CanManageScenario(h.db, h.groupService, scenario, ctx.UserID)
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
	allowed, err := CanManageScenario(h.db, h.groupService, scenario, ctx.UserID)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !allowed {
		return utils.PermissionDeniedError(action, entityLabel)
	}
	return nil
}
