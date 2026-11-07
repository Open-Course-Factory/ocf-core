package scenarioHooks

import (
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"
)

// ScenarioStepQuizScoringHook refuses an unknown quiz scoring mode or an
// out-of-range wrong-answer penalty. Binding tags are inert on entity routes
// (#390), so the rule from models.QuizScoringProblem is enforced here.
type ScenarioStepQuizScoringHook struct {
	hooks.BaseHook
}

func NewScenarioStepQuizScoringHook() hooks.Hook {
	return &ScenarioStepQuizScoringHook{
		BaseHook: hooks.BaseHook{
			Name:       "scenario_step_quiz_scoring",
			EntityName: "ScenarioStep",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate},
			Enabled:    true,
			Priority:   20, // after scenario_step_authorization
		},
	}
}

// Execute reads the step on create and the patch map on update; a field
// absent from the patch keeps its stored, already-valid value.
func (h *ScenarioStepQuizScoringHook) Execute(ctx *hooks.HookContext) error {
	var mode string
	var penalty *float64
	switch v := ctx.NewEntity.(type) {
	case *models.ScenarioStep:
		mode, penalty = v.QuizScoringMode, v.QuizWrongPenalty
	case map[string]any:
		mode, _ = v["quiz_scoring_mode"].(string)
		if p, ok := v["quiz_wrong_penalty"].(float64); ok {
			penalty = &p
		}
	default:
		return nil
	}
	if problem := models.QuizScoringProblem(mode, penalty); problem != "" {
		return entityErrors.NewValidationError("quiz_scoring", problem)
	}
	return nil
}
