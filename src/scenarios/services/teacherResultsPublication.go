package services

import (
	"time"

	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
)

// PublishScenarioResults releases the class's exam grades for a scenario to its
// learners (see WithheldGradeSessions) and returns when it did.
func (s *TeacherDashboardService) PublishScenarioResults(groupID, scenarioID uuid.UUID) (*time.Time, error) {
	now := time.Now()
	if err := s.setResultsPublishedAt(groupID, scenarioID, &now); err != nil {
		return nil, err
	}
	return &now, nil
}

// UnpublishScenarioResults withholds the class's exam grades again.
func (s *TeacherDashboardService) UnpublishScenarioResults(groupID, scenarioID uuid.UUID) error {
	return s.setResultsPublishedAt(groupID, scenarioID, nil)
}

// setResultsPublishedAt writes every assignment of the scenario to the group,
// so a duplicated assignment cannot keep releasing grades after an unpublish.
func (s *TeacherDashboardService) setResultsPublishedAt(groupID, scenarioID uuid.UUID, at *time.Time) error {
	result := s.db.Model(&models.ScenarioAssignment{}).
		Where("group_id = ? AND scenario_id = ?", groupID, scenarioID).
		Update("results_published_at", at)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrScenarioNotAssignedToGroup
	}
	return nil
}
