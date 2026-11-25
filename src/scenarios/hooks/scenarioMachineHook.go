package scenarioHooks

import (
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"
)

// ScenarioMachineHook refuses an unknown machine on the generic create and
// edit routes; the import paths check the same rule in ScenarioContentProblems.
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
	var machine string
	switch v := ctx.NewEntity.(type) {
	case *models.Scenario:
		machine = v.Machine
	case map[string]any:
		switch m := v["machine"].(type) {
		case string:
			machine = m
		case *string:
			if m != nil {
				machine = *m
			}
		}
	default:
		return nil
	}
	if problem := models.ScenarioMachineProblem(machine); problem != "" {
		return entityErrors.NewValidationError("machine", problem)
	}
	return nil
}
