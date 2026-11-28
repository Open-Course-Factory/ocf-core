package scenarioHooks

import (
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"
)

// ScenarioMachineHook refuses an unknown machine or display, an app no
// scenario may offer on it, or a boot override a scenario may not set, on the
// generic create and edit routes; the import paths check the same rules
// (Scenario.MachineProblems) in ScenarioContentProblems.
type ScenarioMachineHook struct {
	hooks.BaseHook
}

func NewScenarioMachineHook() hooks.Hook {
	return &ScenarioMachineHook{
		BaseHook: hooks.BaseHook{
			Name:       "scenario_machine",
			EntityName: "Scenario",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate},
			Enabled:    true,
			Priority:   20, // after scenario_authorization
		},
	}
}

func (h *ScenarioMachineHook) Execute(ctx *hooks.HookContext) error {
	var scenario models.Scenario
	switch v := ctx.NewEntity.(type) {
	case *models.Scenario:
		scenario = *v
	case map[string]any:
		// DtoToMap dereferences the EditDto's pointers, so a patched value is a string.
		scenario.Machine, _ = v["machine"].(string)
		scenario.MachineApps, _ = v["machine_apps"].(string)
		scenario.MachineDisplay, _ = v["machine_display"].(string)
		scenario.MachineBoot, _ = v["machine_boot"].(string)
	default:
		return nil
	}
	if problems := scenario.MachineProblems(); len(problems) > 0 {
		return entityErrors.NewValidationError(problems[0].Field, problems[0].Problem)
	}
	return nil
}
