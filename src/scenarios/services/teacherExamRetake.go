package services

import (
	"fmt"
	"time"

	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// markExamLockedResults flags the rows of learners who used their exam
// attempt, in one query for the page.
func markExamLockedResults(db *gorm.DB, scenarioID uuid.UUID, results []ScenarioResultItem) error {
	userIDs := make([]string, 0, len(results))
	for _, r := range results {
		userIDs = append(userIDs, r.UserID)
	}
	locked, err := ExamLockedRuns(db, []uuid.UUID{scenarioID}, userIDs)
	if err != nil {
		return err
	}
	for i := range results {
		results[i].ExamLocked = locked[learnerRun{ScenarioID: scenarioID, UserID: results[i].UserID}]
	}
	return nil
}

// AllowRetake releases the learner of a class session from their used exam
// attempt so they can start one more run. Every run holding the lock is
// marked, so none is left to keep them out; the runs and their grades stay.
// The session must belong to the class the same way GetSessionDetail checks.
func (s *TeacherDashboardService) AllowRetake(groupID, sessionID uuid.UUID) (*time.Time, error) {
	session, err := s.classSession(groupID, sessionID)
	if err != nil {
		return nil, err
	}
	ids, err := lockingSessionIDs(s.db, session.UserID, session.ScenarioID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, ErrExamNotLocked
	}
	now := time.Now()
	if err := s.db.Model(&models.ScenarioSession{}).Where("id IN ?", ids).
		Update("retake_allowed_at", now).Error; err != nil {
		return nil, err
	}
	return &now, nil
}

// classSession loads a session through the class access gate of
// GetSessionDetails: its learner is an active member and its scenario is
// assigned to the class. Any refusal is ErrSessionNotInGroup, so a caller
// cannot tell a foreign session from a missing one.
func (s *TeacherDashboardService) classSession(groupID, sessionID uuid.UUID) (*models.ScenarioSession, error) {
	sessions, err := s.loadSessionsForBulkDetails([]uuid.UUID{sessionID})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSessionNotInGroup, err)
	}
	session := sessions[0]
	if err := s.verifyGroupAccessForBulkDetails(groupID, []string{session.UserID}, []uuid.UUID{session.ScenarioID}); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSessionNotInGroup, err)
	}
	return &session, nil
}
