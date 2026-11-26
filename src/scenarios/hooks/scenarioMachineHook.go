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
		machine, machineApps = stringField(v, "machine"), stringField(v, "machine_apps")
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

// stringField reads an update map's string value, given as a string or a
// *string; absent reads as "", which both checks accept.
func stringField(updates map[string]any, key string) string {
	switch v := updates[key].(type) {
	case string:
		return v
	case *string:
		if v != nil {
			return *v
		}
	}
	return ""
}
