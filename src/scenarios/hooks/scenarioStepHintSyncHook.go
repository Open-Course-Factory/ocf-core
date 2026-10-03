package scenarioHooks

import (
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"

	"gorm.io/gorm"
)

// ScenarioStepHintSyncHook rebuilds a step's progressive hint rows when its
// hint_content is edited. Once rows exist the learner only ever sees the rows,
// so a step whose hint was edited kept serving the hints it was created with.
//
// It compares against the stored hint so a save that resends the same text
// leaves the rows alone, including rows an administrator edited one by one.
type ScenarioStepHintSyncHook struct {
	db *gorm.DB
	hooks.BaseHook
}

func NewScenarioStepHintSyncHook(db *gorm.DB) *ScenarioStepHintSyncHook {
	return &ScenarioStepHintSyncHook{
		db: db,
		// After the authorization hook (priority 10): nothing is rewritten for
		// a caller who may not edit the step.
		BaseHook: hooks.BaseHook{
			Name:       "scenario_step_hint_sync",
			EntityName: "ScenarioStep",
			HookTypes:  []hooks.HookType{hooks.BeforeUpdate},
			Enabled:    true,
			Priority:   20,
		},
	}
}

func (h *ScenarioStepHintSyncHook) Execute(ctx *hooks.HookContext) error {
	updates, ok := ctx.NewEntity.(map[string]any)
	if !ok {
		return nil
	}
	hint, written := updates["hint_content"].(string)
	old, ok := ctx.OldEntity.(*models.ScenarioStep)
	if !written || !ok || hint == old.HintContent {
		return nil
	}
	return h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("step_id = ?", old.ID).Delete(&models.ScenarioStepHint{}).Error; err != nil {
			return fmt.Errorf("delete hints of step %s: %w", old.ID, err)
		}
		hints := services.BuildStepHints(hint)
		for i := range hints {
			hints[i].StepID = old.ID
		}
		if len(hints) == 0 {
			return nil
		}
		if err := tx.Create(&hints).Error; err != nil {
			return fmt.Errorf("create hints of step %s: %w", old.ID, err)
		}
		return nil
	})
}
