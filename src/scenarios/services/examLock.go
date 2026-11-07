package services

import (
	"errors"

	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrExamAlreadySubmitted refuses a new run, or the abandon of the current
// one, for a learner who has used their exam attempt on this scenario.
var ErrExamAlreadySubmitted = errors.New("this exam has already been submitted; ask your teacher to allow a retake")

// ErrExamNotLocked refuses a retake for a learner who has an attempt left.
var ErrExamNotLocked = errors.New("this learner has not submitted this exam, so there is nothing to release")

// ExamLockReason is the stable code clients switch on for ErrExamAlreadySubmitted.
const ExamLockReason = "exam_already_submitted"

// examLockingSessions is the one rule for the exam's single attempt: a
// learner's non-preview run in which an exam-mode quiz step was submitted
// (its score recorded), and that the teacher has not released for a retake.
// Any such run locks the learner out of starting another run of the scenario.
func examLockingSessions(db *gorm.DB, scenarioIDs []uuid.UUID, userIDs []string) *gorm.DB {
	return db.Table("scenario_sessions ss").
		Joins("JOIN scenario_step_progress sp ON sp.session_id = ss.id AND sp.quiz_score IS NOT NULL AND sp.deleted_at IS NULL").
		Joins(`JOIN scenario_steps st ON st.scenario_id = ss.scenario_id AND st."order" = sp.step_order AND st.deleted_at IS NULL`).
		Scopes(examQuizStep("st")).
		Where("ss.scenario_id IN ? AND ss.user_id IN ? AND ss.is_preview = ? AND ss.retake_allowed_at IS NULL AND ss.deleted_at IS NULL",
			scenarioIDs, userIDs, false)
}

// ExamLockedRuns returns the (scenario, learner) pairs that are locked, in one
// query whatever the number of each. An empty input locks nothing.
func ExamLockedRuns(db *gorm.DB, scenarioIDs []uuid.UUID, userIDs []string) (map[learnerRun]bool, error) {
	locked := map[learnerRun]bool{}
	if len(scenarioIDs) == 0 || len(userIDs) == 0 {
		return locked, nil
	}
	var rows []learnerRun
	if err := examLockingSessions(db, scenarioIDs, userIDs).
		Select("DISTINCT ss.scenario_id AS scenario_id, ss.user_id AS user_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		locked[r] = true
	}
	return locked, nil
}

// ExamLockedScenarios is ExamLockedRuns for one learner: the scenarios they
// may not start again.
func ExamLockedScenarios(db *gorm.DB, userID string, scenarioIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	runs, err := ExamLockedRuns(db, scenarioIDs, []string{userID})
	if err != nil {
		return nil, err
	}
	locked := make(map[uuid.UUID]bool, len(runs))
	for r := range runs {
		locked[r.ScenarioID] = true
	}
	return locked, nil
}

// ExamLocked is ExamLockedRuns for one learner and one scenario.
func ExamLocked(db *gorm.DB, userID string, scenarioID uuid.UUID) (bool, error) {
	locked, err := ExamLockedRuns(db, []uuid.UUID{scenarioID}, []string{userID})
	if err != nil {
		return false, err
	}
	return locked[learnerRun{ScenarioID: scenarioID, UserID: userID}], nil
}

// RefuseIfExamLocked returns ErrExamAlreadySubmitted when the learner may not
// start or abandon a run of the scenario.
func RefuseIfExamLocked(db *gorm.DB, userID string, scenarioID uuid.UUID) error {
	locked, err := ExamLocked(db, userID, scenarioID)
	if err != nil {
		return err
	}
	if locked {
		return ErrExamAlreadySubmitted
	}
	return nil
}

// lockingSessionIDs lists a learner's runs of the scenario that hold the lock.
func lockingSessionIDs(db *gorm.DB, userID string, scenarioID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := examLockingSessions(db, []uuid.UUID{scenarioID}, []string{userID}).
		Distinct().Pluck("ss.id", &ids).Error
	return ids, err
}

// lockedRunOf reports whether the run's learner is locked on its scenario;
// a preview is never locked.
func lockedRunOf(locked map[learnerRun]bool, s models.ScenarioSession) bool {
	return !s.IsPreview && locked[learnerRun{ScenarioID: s.ScenarioID, UserID: s.UserID}]
}
