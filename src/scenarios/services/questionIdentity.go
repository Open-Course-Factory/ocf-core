package services

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
)

// questionKey is what makes a republished question the same question: the step
// it sits in, its place in that step, and its type. A changed type means an old
// answer no longer reads the same way, so that question gets a new id.
type questionKey struct {
	stepOrder     int
	questionOrder int
	questionType  string
}

// replaceQuestionsKeepingIDs prepares a republish of a scenario's questions:
// every incoming question that is the same question as an existing one takes
// its id, then the existing rows are removed for the caller to create the new
// ones.
//
// Learners' answers are stored by question id, so a republish that renumbered
// the questions silently orphaned every answer already given in a running
// session: the answers file a later step reads, the teacher dashboard and the
// quiz replay all lost them. The old rows are deleted for good, not soft-deleted,
// so their ids can be created again.
func replaceQuestionsKeepingIDs(tx *gorm.DB, scenarioID uuid.UUID, steps []models.ScenarioStep) error {
	var existing []struct {
		ID           uuid.UUID
		StepOrder    int
		Order        int
		QuestionType string
	}
	if err := tx.Table("scenario_step_questions AS q").
		Select(`q.id, s."order" AS step_order, q."order", q.question_type`).
		Joins("JOIN scenario_steps s ON s.id = q.step_id").
		Where("s.scenario_id = ? AND s.deleted_at IS NULL AND q.deleted_at IS NULL", scenarioID).
		Scan(&existing).Error; err != nil {
		return fmt.Errorf("failed to load existing questions: %w", err)
	}
	ids := make(map[questionKey]uuid.UUID, len(existing))
	for _, q := range existing {
		ids[questionKey{q.StepOrder, q.Order, q.QuestionType}] = q.ID
	}
	for i := range steps {
		for j := range steps[i].Questions {
			q := &steps[i].Questions[j]
			key := questionKey{steps[i].Order, q.Order, q.QuestionType}
			if id, ok := ids[key]; ok {
				q.ID = id
				delete(ids, key) // once: two questions sharing an order must not share an id
			}
		}
	}

	if err := tx.Unscoped().Where("step_id IN (?)",
		tx.Model(&models.ScenarioStep{}).Select("id").Where("scenario_id = ?", scenarioID),
	).Delete(&models.ScenarioStepQuestion{}).Error; err != nil {
		return fmt.Errorf("failed to delete old questions: %w", err)
	}
	return nil
}
