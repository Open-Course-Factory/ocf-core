package scenarioHooks

import (
	"errors"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"

	"gorm.io/gorm"
)

// ScenarioSessionExamLockHook refuses the generic session create for a
// learner who used their exam attempt. Members may POST sessions through the
// entity route, so without it that route is a launch that skips the lock.
type ScenarioSessionExamLockHook struct {
	db *gorm.DB
	hooks.BaseHook
}

func NewScenarioSessionExamLockHook(db *gorm.DB) hooks.Hook {
	return &ScenarioSessionExamLockHook{
		db: db,
		BaseHook: hooks.BaseHook{
			Name:       "scenario_session_exam_lock",
			EntityName: "ScenarioSession",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate},
			Enabled:    true,
			Priority:   20,
		},
	}
}

func (h *ScenarioSessionExamLockHook) Execute(ctx *hooks.HookContext) error {
	session, ok := ctx.NewEntity.(*models.ScenarioSession)
	if !ok || session.IsPreview {
		return nil
	}
	err := services.RefuseIfExamLocked(h.db, session.UserID, session.ScenarioID)
	if errors.Is(err, services.ErrExamAlreadySubmitted) {
		return entityErrors.NewConflictError(services.ExamLockReason)
	}
	return err
}
