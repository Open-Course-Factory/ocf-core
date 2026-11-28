package services

import (
	"errors"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrScenarioNeedsContainer refuses a run that would go to the learner's
// browser although its result must not be in the learner's hands: an exam, or a
// crash-trap scenario, whose config file holds every flag of the run. A learner
// who owns the machine can read every file of it and fake every check run in
// it. Flag steps do run locally (decided 2026-11-25): one step's flag is no
// more exposed than to a root learner in a container.
var ErrScenarioNeedsContainer = errors.New("this scenario must run in a container: its checks cannot be trusted on a machine in your browser")

// MachineContext is everything that decides where a run goes.
// ErrScenarioNeedsDesktop refuses a new run of a desktop scenario while the
// platform's desktop switch is off.
var ErrScenarioNeedsDesktop = errors.New("this scenario shows a desktop in your browser, which is not available yet")

// LaunchableDisplays drops the scenarios no one may launch because their
// desktop is switched off: the launcher never offers what RunMachine refuses.
func LaunchableDisplays(db *gorm.DB, scenarios []models.Scenario) []models.Scenario {
	if terminalServices.LocalMachineDesktopEnabled(db) {
		return scenarios
	}
	kept := scenarios[:0]
	for _, s := range scenarios {
		if !s.NeedsDesktop() {
			kept = append(kept, s)
		}
	}
	return kept
}

type MachineContext struct {
	// SwitchOn is the platform switch (LocalMachinesEnabled).
	SwitchOn bool
	// PlanSetting is the learner's plan's LocalMachines value.
	PlanSetting string
	// ScenarioMachine is the scenario's Machine value.
	ScenarioMachine string
	// Assigned: the run comes from a class or organization assignment.
	Assigned bool
	// What the learner could fake if they owned the machine.
	Exam, CrashTraps bool
}

// ScenarioMachine decides where a run goes: models.ScenarioMachineIncus or
// models.ScenarioMachineLocal. It is the one place for the rule; RunMachine
// reads its inputs.
func ScenarioMachine(ctx MachineContext) (string, error) {
	if !wantsLocal(ctx) {
		return models.ScenarioMachineIncus, nil
	}
	if ctx.Exam || ctx.CrashTraps {
		// By account accepts either machine, so it simply gets the container.
		if ctx.ScenarioMachine == models.ScenarioMachineByAccount {
			return models.ScenarioMachineIncus, nil
		}
		return "", ErrScenarioNeedsContainer
	}
	return models.ScenarioMachineLocal, nil
}

// wantsLocal is the plan's table, behind the platform switch. A class or
// organization assignment keeps a By account scenario on Incus, where it can be
// supervised; a Local scenario stays local wherever it comes from.
func wantsLocal(ctx MachineContext) bool {
	if !ctx.SwitchOn {
		return false
	}
	switch ctx.ScenarioMachine {
	case models.ScenarioMachineLocal:
		return ctx.PlanSetting == paymentModels.LocalMachinesAllowed ||
			ctx.PlanSetting == paymentModels.LocalMachinesByAccountDefault
	case models.ScenarioMachineByAccount:
		return !ctx.Assigned && ctx.PlanSetting == paymentModels.LocalMachinesByAccountDefault
	}
	return false
}

// RunMachine decides where a new run of scenario goes for a learner on plan
// (nil: no plan, so off), assigned saying whether a class or organization
// assignment brought it. A run keeps its machine until it ends: this is for
// new runs only.
func RunMachine(db *gorm.DB, scenario *models.Scenario, plan *paymentModels.SubscriptionPlan, assigned bool) (string, error) {
	if scenario.NeedsDesktop() && !terminalServices.LocalMachineDesktopEnabled(db) {
		return "", ErrScenarioNeedsDesktop
	}
	ctx := MachineContext{
		SwitchOn:        terminalServices.LocalMachinesEnabled(db),
		ScenarioMachine: scenario.Machine,
		Assigned:        assigned,
		CrashTraps:      scenario.CrashTraps,
	}
	if plan != nil {
		ctx.PlanSetting = plan.LocalMachines
	}
	if wantsLocal(ctx) {
		exams, err := ExamScenarios(db, []uuid.UUID{scenario.ID})
		if err != nil {
			return "", err
		}
		ctx.Exam = exams[scenario.ID]
	}
	return ScenarioMachine(ctx)
}

// MachineOfTerminal is where a run on terminalSessionID runs: the host that
// created the session named it.
func MachineOfTerminal(terminalSessionID string) string {
	if terminalModels.IsLocalSessionID(terminalSessionID) {
		return models.ScenarioMachineLocal
	}
	return models.ScenarioMachineIncus
}
