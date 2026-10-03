package scenarioHooks

import (
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"
)

// ScenarioStepFlagHook applies models.NormalizeFlagStep to a step PATCH. The
// PATCH writes a column map, which the model's BeforeSave never sees.
//
// has_flag only counts when the request sends it: a PATCH that turns a flag
// step into a terminal step must lose the flag the row still carries.
type ScenarioStepFlagHook struct {
	hooks.BaseHook
}

func NewScenarioStepFlagHook() hooks.Hook {
	return &ScenarioStepFlagHook{
		BaseHook: hooks.BaseHook{
			Name:       "scenario_step_flag",
			EntityName: "ScenarioStep",
			HookTypes:  []hooks.HookType{hooks.BeforeUpdate},
			Enabled:    true,
			Priority:   20, // after scenario_step_authorization
		},
	}
}

func (h *ScenarioStepFlagHook) Execute(ctx *hooks.HookContext) error {
	updates, ok := ctx.NewEntity.(map[string]any)
	if !ok {
		return nil
	}
	old, ok := ctx.OldEntity.(*models.ScenarioStep)
	if !ok {
		return nil
	}
	stepType, typeSent := updates["step_type"].(string)
	hasFlag, flagSent := updates["has_flag"].(bool)
	if !typeSent && !flagSent {
		return nil
	}
	if !typeSent {
		stepType = old.StepType
	}
	updates["step_type"], updates["has_flag"] = models.NormalizeFlagStep(stepType, hasFlag)
	return nil
}
