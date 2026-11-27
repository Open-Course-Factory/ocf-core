package scenarioHooks

import (
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"
)

// ScenarioMachineHook refuses an unknown machine or display, an app no
// scenario may offer on it, or a boot override a scenario may not set, on the
// generic create and edit routes; the import paths check
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
	var machine, machineApps, machineDisplay, machineBoot string
	switch v := ctx.NewEntity.(type) {
	case *models.Scenario:
		machine, machineApps, machineDisplay, machineBoot = v.Machine, v.MachineApps, v.MachineDisplay, v.MachineBoot
	case map[string]any:
		// DtoToMap dereferences the EditDto's pointers, so a patched value is a string.
		machine, _ = v["machine"].(string)
		machineApps, _ = v["machine_apps"].(string)
		machineDisplay, _ = v["machine_display"].(string)
		machineBoot, _ = v["machine_boot"].(string)
	default:
		return nil
	}
	if problem := models.ScenarioMachineProblem(machine); problem != "" {
		return entityErrors.NewValidationError("machine", problem)
	}
	if problem := models.ScenarioMachineAppsProblem(machineApps); problem != "" {
		return entityErrors.NewValidationError("machine_apps", problem)
	}
	if problem := models.ScenarioMachineDisplayProblem(machineDisplay); problem != "" {
		return entityErrors.NewValidationError("machine_display", problem)
	}
	if problem := models.ScenarioMachineBootProblem(machineBoot); problem != "" {
		return entityErrors.NewValidationError("machine_boot", problem)
	}
	return nil
}
