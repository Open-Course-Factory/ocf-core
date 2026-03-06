package services

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
)

// GroupActivityItem represents an active session for a group member
type GroupActivityItem struct {
	SessionID         uuid.UUID `json:"session_id"`
	UserID            string    `json:"user_id"`
	ScenarioID        uuid.UUID `json:"scenario_id"`
	ScenarioTitle     string    `json:"scenario_title"`
	CurrentStep       int       `json:"current_step"`
	TotalSteps        int64     `json:"total_steps"`
	Status            string    `json:"status"`
	StartedAt         time.Time `json:"started_at"`
	TerminalSessionID *string   `json:"terminal_session_id,omitempty"`
}

// ScenarioResultItem represents a single session result for a scenario
type ScenarioResultItem struct {
	SessionID   uuid.UUID  `json:"session_id"`
	UserID      string     `json:"user_id"`
	Status      string     `json:"status"`
	Grade       *float64   `json:"grade,omitempty"`
	CurrentStep int        `json:"current_step"`
	TotalSteps  int64      `json:"total_steps"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ScenarioAnalytics represents aggregated analytics for a scenario within a group
type ScenarioAnalytics struct {
	TotalSessions         int64    `json:"total_sessions"`
	CompletedCount        int64    `json:"completed_count"`
	CompletionRate        float64  `json:"completion_rate"`
	AvgGrade              *float64 `json:"avg_grade,omitempty"`
	AvgCompletionTimeSecs *float64 `json:"avg_completion_time_seconds,omitempty"`
}

// TeacherDashboardService provides teacher-facing queries for group activity and scenario results
type TeacherDashboardService struct {
	db *gorm.DB
}

// NewTeacherDashboardService creates a new teacher dashboard service
func NewTeacherDashboardService(db *gorm.DB) *TeacherDashboardService {
	return &TeacherDashboardService{db: db}
}

// GetGroupActivity returns active sessions for all members of a group (single JOIN query, no N+1)
func (s *TeacherDashboardService) GetGroupActivity(groupID uuid.UUID) ([]GroupActivityItem, error) {
	var results []GroupActivityItem
	err := s.db.Raw(`
		SELECT ss.id as session_id, ss.user_id, ss.current_step, ss.status, ss.started_at, ss.terminal_session_id,
		       sc.title as scenario_title, sc.id as scenario_id,
		       (SELECT COUNT(*) FROM scenario_steps WHERE scenario_id = sc.id) as total_steps
		FROM scenario_sessions ss
		JOIN scenarios sc ON sc.id = ss.scenario_id
		JOIN group_members gm ON gm.user_id = ss.user_id AND gm.group_id = ? AND gm.is_active = true
		WHERE ss.status = 'active'
		ORDER BY ss.started_at DESC
	`, groupID).Scan(&results).Error
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []GroupActivityItem{}
	}
	return results, nil
}

// GetScenarioResults returns all sessions for a specific scenario within a group (single JOIN query, no N+1)
func (s *TeacherDashboardService) GetScenarioResults(groupID, scenarioID uuid.UUID) ([]ScenarioResultItem, error) {
	var results []ScenarioResultItem
	err := s.db.Raw(`
		SELECT ss.id as session_id, ss.user_id, ss.status, ss.grade, ss.started_at, ss.completed_at, ss.current_step,
		       (SELECT COUNT(*) FROM scenario_steps WHERE scenario_id = ss.scenario_id) as total_steps
		FROM scenario_sessions ss
		JOIN group_members gm ON gm.user_id = ss.user_id AND gm.group_id = ? AND gm.is_active = true
		WHERE ss.scenario_id = ?
		ORDER BY ss.started_at DESC
	`, groupID, scenarioID).Scan(&results).Error
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []ScenarioResultItem{}
	}
	return results, nil
}

// GetScenarioAnalytics computes aggregate statistics for a scenario within a group.
// Calculations are done in Go to avoid SQLite vs PostgreSQL syntax differences.
func (s *TeacherDashboardService) GetScenarioAnalytics(groupID, scenarioID uuid.UUID) (*ScenarioAnalytics, error) {
	results, err := s.GetScenarioResults(groupID, scenarioID)
	if err != nil {
		return nil, err
	}

	analytics := &ScenarioAnalytics{}
	analytics.TotalSessions = int64(len(results))

	var gradeSum float64
	var gradeCount int64
	var timeSum float64
	var timeCount int64

	for _, r := range results {
		if r.Status == "completed" {
			analytics.CompletedCount++
			if r.Grade != nil {
				gradeSum += *r.Grade
				gradeCount++
			}
			if r.CompletedAt != nil {
				duration := r.CompletedAt.Sub(r.StartedAt).Seconds()
				timeSum += duration
				timeCount++
			}
		}
	}

	if analytics.TotalSessions > 0 {
		analytics.CompletionRate = float64(analytics.CompletedCount) / float64(analytics.TotalSessions) * 100.0
	}

	if gradeCount > 0 {
		avgGrade := gradeSum / float64(gradeCount)
		analytics.AvgGrade = &avgGrade
	}

	if timeCount > 0 {
		avgTime := timeSum / float64(timeCount)
		analytics.AvgCompletionTimeSecs = &avgTime
	}

	return analytics, nil
}

// BulkStartResult represents the result of a bulk start operation
type BulkStartResult struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
}

// CalculateGrade computes the grade for a session as percentage of completed steps
func (s *TeacherDashboardService) CalculateGrade(sessionID uuid.UUID) float64 {
	var session models.ScenarioSession
	if err := s.db.First(&session, "id = ?", sessionID).Error; err != nil {
		return 0
	}

	var totalSteps int64
	s.db.Model(&models.ScenarioStep{}).Where("scenario_id = ?", session.ScenarioID).Count(&totalSteps)
	if totalSteps == 0 {
		return 0
	}

	var completedSteps int64
	s.db.Model(&models.ScenarioStepProgress{}).Where("session_id = ? AND status = ?", sessionID, "completed").Count(&completedSteps)

	return (float64(completedSteps) / float64(totalSteps)) * 100
}

// BulkStartScenario creates sessions for group members who don't already have an active one
func (s *TeacherDashboardService) BulkStartScenario(groupID uuid.UUID, scenarioID uuid.UUID) (*BulkStartResult, error) {
	var members []groupModels.GroupMember
	if err := s.db.Where("group_id = ? AND is_active = ?", groupID, true).Find(&members).Error; err != nil {
		return nil, fmt.Errorf("failed to load group members: %w", err)
	}

	// Verify scenario exists and has steps
	var scenario models.Scenario
	if err := s.db.Preload("Steps").First(&scenario, "id = ?", scenarioID).Error; err != nil {
		return nil, fmt.Errorf("scenario not found: %w", err)
	}

	result := &BulkStartResult{}

	for _, member := range members {
		// Check for existing active session
		var existing models.ScenarioSession
		err := s.db.Where("user_id = ? AND scenario_id = ? AND status IN ?",
			member.UserID, scenarioID, []string{"active", "in_progress"}).First(&existing).Error
		if err == nil {
			result.Skipped++
			continue
		}

		now := time.Now()
		session := models.ScenarioSession{
			ScenarioID: scenarioID,
			UserID:     member.UserID,
			Status:     "active",
			StartedAt:  now,
		}
		if err := s.db.Create(&session).Error; err != nil {
			return nil, fmt.Errorf("failed to create session for user %s: %w", member.UserID, err)
		}

		// Create step progress records
		for _, step := range scenario.Steps {
			status := "locked"
			if step.Order == 0 {
				status = "active"
			}
			progress := models.ScenarioStepProgress{
				SessionID: session.ID,
				StepOrder: step.Order,
				Status:    status,
			}
			if err := s.db.Create(&progress).Error; err != nil {
				return nil, fmt.Errorf("failed to create step progress: %w", err)
			}
		}

		result.Created++
	}

	return result, nil
}
