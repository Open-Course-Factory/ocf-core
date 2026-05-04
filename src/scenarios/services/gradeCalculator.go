package services

import (
	"fmt"
	"math"

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

// ComputeCorrectCountsFromLoaded is a pure function that returns the absolute
// count of correct answers (numerator) and the total possible (denominator)
// for a session. The denominator is static per scenario — it counts every
// quiz question in every quiz step plus every flag-bearing step, regardless
// of session progress. This means in-progress sessions show their absolute
// progress (e.g. 3/15) rather than a ratio of attempted (3/5).
//
// Numerator semantics:
//   - quiz step: math.Round(QuizScore * questionCount) when progress has a
//     non-nil QuizScore; 0 otherwise. math.Round avoids float drift
//     (e.g. 0.6*5 = 2.9999... must yield 3, not 2).
//   - flag-bearing step: 1 if a ScenarioFlag row for that step exists with
//     IsCorrect=true; 0 otherwise (including missing row).
//   - terminal/info steps: ignored entirely.
//
// Denominator semantics:
//   - quiz step: + questionCountByStepID[step.ID]
//   - flag-bearing step (StepType=="flag" OR HasFlag): + 1
//   - terminal/info steps: ignored entirely.
//
// Callers must pre-filter `steps` to exclude soft-deleted rows and
// `questionCountByStepID` to exclude soft-deleted questions.
func ComputeCorrectCountsFromLoaded(
	steps []models.ScenarioStep,
	progress []models.ScenarioStepProgress,
	flags []models.ScenarioFlag,
	questionCountByStepID map[uuid.UUID]int,
) (correct int64, total int64) {
	// Index progress and flags by step_order for O(1) lookup. Both models key
	// off StepOrder (not StepID) because they predate the per-step UUID
	// linkage.
	progressByOrder := make(map[int]models.ScenarioStepProgress, len(progress))
	for _, p := range progress {
		progressByOrder[p.StepOrder] = p
	}
	flagByOrder := make(map[int]models.ScenarioFlag, len(flags))
	for _, f := range flags {
		// Prefer a correct submission if multiple rows exist for the same step.
		if existing, ok := flagByOrder[f.StepOrder]; ok && existing.IsCorrect {
			continue
		}
		flagByOrder[f.StepOrder] = f
	}

	for _, step := range steps {
		stepType := normalizeStepType(step.StepType)
		switch {
		case stepType == "quiz":
			n := questionCountByStepID[step.ID]
			if n == 0 {
				continue
			}
			total += int64(n)
			if p, ok := progressByOrder[step.Order]; ok && p.QuizScore != nil {
				correct += int64(math.Round(*p.QuizScore * float64(n)))
			}
		case stepType == "flag" || step.HasFlag:
			total += 1
			if f, ok := flagByOrder[step.Order]; ok && f.IsCorrect {
				correct += 1
			}
		}
	}

	return correct, total
}
