package scenarioHooks

import (
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"
)

// ScenarioMachineHook refuses an unknown machine, or an app no scenario may
// offer on it, on the generic create and edit routes; the import paths check
// the same rules in ScenarioContentProblems.
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
	var machine, machineApps string
	switch v := ctx.NewEntity.(type) {
	case *models.Scenario:
		machine, machineApps = v.Machine, v.MachineApps
	case map[string]any:
		// DtoToMap dereferences the EditDto's pointers, so a patched value is a string.
		machine, _ = v["machine"].(string)
		machineApps, _ = v["machine_apps"].(string)
	default:
		return nil
	}
	if problem := models.ScenarioMachineProblem(machine); problem != "" {
		return entityErrors.NewValidationError("machine", problem)
	}
	if problem := models.ScenarioMachineAppsProblem(machineApps); problem != "" {
		return entityErrors.NewValidationError("machine_apps", problem)
	}
	return nil
}
