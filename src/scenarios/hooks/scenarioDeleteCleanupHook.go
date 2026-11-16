package scenarioHooks

import (
	"fmt"
	"log/slog"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"
	scenarioServices "soli/formations/src/scenarios/services"
	ttServices "soli/formations/src/terminalTrainer/services"

	"gorm.io/gorm"
)

// ScenarioDeleteCleanupHook gives the generic DELETE /scenarios/:id the same
// cleanup as the organisation delete route: open runs abandoned with their
// terminals released, assignments dropped.
type ScenarioDeleteCleanupHook struct {
	db             *gorm.DB
	deleteTerminal scenarioServices.TerminalDeleteFunc
	hooks.BaseHook
}

// NewScenarioDeleteCleanupHook builds the scenario delete cleanup hook.
func NewScenarioDeleteCleanupHook(db *gorm.DB) hooks.Hook {
	return &ScenarioDeleteCleanupHook{
		db:             db,
		deleteTerminal: ttServices.NewTerminalTrainerService(db).DeleteSession,
		BaseHook: hooks.BaseHook{
			Name:       "scenario_delete_cleanup",
			EntityName: "Scenario",
			HookTypes:  []hooks.HookType{hooks.AfterDelete},
			Enabled:    true,
			Priority:   10,
		},
	}
}

func (h *ScenarioDeleteCleanupHook) Execute(ctx *hooks.HookContext) error {
	scenario, ok := ctx.NewEntity.(*models.Scenario)
	if !ok {
		slog.Warn("AfterDelete: expected *models.Scenario", "got", fmt.Sprintf("%T", ctx.NewEntity))
		return nil
	}
	// shortcut: cleanup runs after the delete and its failure is only logged, move it into the delete transaction if dangling runs are ever observed
	runs, err := scenarioServices.CleanUpDeletedScenario(h.db, scenario.ID)
	if err != nil {
		slog.Error("AfterDelete: scenario cleanup failed", "scenario_id", scenario.ID, "error", err)
		return nil
	}
	scenarioServices.ReleaseTerminals(h.db, h.deleteTerminal, runs)
	return nil
}
