package services

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
)

// QuizScoreOverride lets callers override the quiz score for a specific step
// when computing the weighted grade. It is intended for cases where the just-
// submitted quiz score has been persisted to DB but the in-memory step
// progress slice is stale (e.g. inside advanceToNextStep right after
// SubmitQuiz wrote the row). Both fields must be set.
type QuizScoreOverride struct {
	StepOrder int
	QuizScore float64
}

// ComputeWeightedGrade returns the weighted grade for a session as a
// percentage in [0,100]. It loads the scenario steps and step progress for the
// given session and delegates the actual computation to
// ComputeWeightedGradeFromLoaded.
//
// Returns 0 when the session has no steps. Errors propagate from the DB
// layer; callers that want best-effort behaviour can ignore the error and
// use the zero return value.
func ComputeWeightedGrade(db *gorm.DB, sessionID uuid.UUID) (float64, error) {
	var session models.ScenarioSession
	if err := db.First(&session, "id = ?", sessionID).Error; err != nil {
		return 0, fmt.Errorf("session not found: %w", err)
	}

	var steps []models.ScenarioStep
	if err := db.Where("scenario_id = ?", session.ScenarioID).
		Order("\"order\" ASC").
		Find(&steps).Error; err != nil {
		return 0, fmt.Errorf("failed to load scenario steps: %w", err)
	}

	var progress []models.ScenarioStepProgress
	if err := db.Where("session_id = ?", sessionID).
		Find(&progress).Error; err != nil {
		return 0, fmt.Errorf("failed to load step progress: %w", err)
	}

	return ComputeWeightedGradeFromLoaded(steps, progress, nil), nil
}

// ComputeWeightedGradeFromLoaded is a pure function that averages the per-step
// weights and returns the result as a percentage in [0,100]. Step weights:
//
//	terminal/flag/info → 1.0 if the step's progress status == "completed",
//	                     else 0.0
//	quiz               → progress.QuizScore (0 if nil)
//
// Legacy rows with an empty step_type are treated as "terminal" so older
// sessions keep their grades.
//
// `currentStepOverride`, when non-nil, overrides the matching step's quiz
// score. This is used by SubmitQuiz to score the just-submitted final step
// before its DB row is reloaded into the in-memory progress slice.
//
// Returns 0 when len(steps) == 0.
func ComputeWeightedGradeFromLoaded(steps []models.ScenarioStep, progress []models.ScenarioStepProgress, currentStepOverride *QuizScoreOverride) float64 {
	totalSteps := len(steps)
	if totalSteps == 0 {
		return 0
	}

	// Index progress by step_order for O(1) lookup.
	progressByOrder := make(map[int]models.ScenarioStepProgress, len(progress))
	for _, p := range progress {
		progressByOrder[p.StepOrder] = p
	}

	var sum float64
	for _, step := range steps {
		stepType := normalizeStepType(step.StepType)
		p, hasProgress := progressByOrder[step.Order]

		switch stepType {
		case "quiz":
			// Apply override if it targets this step.
			if currentStepOverride != nil && currentStepOverride.StepOrder == step.Order {
				sum += currentStepOverride.QuizScore
				continue
			}
			if hasProgress && p.QuizScore != nil {
				sum += *p.QuizScore
			}
			// nil score (not yet submitted) counts as 0.
		default:
			// terminal / flag / info — full credit if completed, else 0.
			if hasProgress && p.Status == "completed" {
				sum += 1.0
			}
		}
	}

	return (sum / float64(totalSteps)) * 100.0
}
