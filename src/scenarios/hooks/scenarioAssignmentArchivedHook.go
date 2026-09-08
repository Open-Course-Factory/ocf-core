package scenarioHooks

import (
	"fmt"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"

	"gorm.io/gorm"
)

// ScenarioAssignmentArchivedHook refuses to assign an archived scenario.
//
// This is a state rule, not a permission, so it applies to platform
// administrators as well: an archived scenario must not become newly reachable
// for anybody. Assignments created before the scenario was archived are left
// in place — they still name the run the learners' results belong to — and the
// launch gates stop them being acted on.
type ScenarioAssignmentArchivedHook struct {
	db *gorm.DB
	hooks.BaseHook
}

func NewScenarioAssignmentArchivedHook(db *gorm.DB) hooks.Hook {
	return &ScenarioAssignmentArchivedHook{
		db: db,
		BaseHook: hooks.BaseHook{
			Name:       "scenario_assignment_archived",
			EntityName: "ScenarioAssignment",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate},
			Enabled:    true,
			Priority:   5,
		},
	}
}

func (h *ScenarioAssignmentArchivedHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType != hooks.BeforeCreate {
		return nil
	}

	assignment, ok := ctx.NewEntity.(*models.ScenarioAssignment)
	if !ok {
		return fmt.Errorf("expected *models.ScenarioAssignment, got %T", ctx.NewEntity)
	}

	var scenario models.Scenario
	if err := h.db.Select("archived_at").First(&scenario, "id = ?", assignment.ScenarioID).Error; err != nil {
		// A missing scenario is the create's own problem to report, not this
		// hook's: refusing here would mask the real error.
		return nil
	}

	if scenario.IsArchived() {
		// Structured so the generic create answers 409 with the scenario
		// wording, not the hook-failure 500; the sentinel stays reachable
		// through errors.Is for the callers that match on it.
		return entityErrors.NewStateConflictError("ScenarioAssignment", models.ErrScenarioArchived)
	}
	return nil
}
