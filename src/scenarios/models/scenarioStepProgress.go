package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// ScenarioStepProgress tracks a user's progress on a specific step within a session
type ScenarioStepProgress struct {
	entityManagementModels.BaseModel
	SessionID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"session_id"`
	StepOrder        int        `gorm:"not null" json:"step_order"`
	Status           string     `gorm:"type:varchar(50);default:'locked'" json:"status"` // locked, active, completed, skipped
	VerifyAttempts   int        `gorm:"default:0" json:"verify_attempts"`
	HintsRevealed    int        `gorm:"default:0" json:"hints_revealed"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	TimeSpentSeconds int        `gorm:"default:0" json:"time_spent_seconds"`
	// StepType is denormalized from ScenarioStep.StepType so the teacher
	// dashboard can filter quiz/info/flag/terminal progress without joining.
	StepType string `gorm:"type:varchar(50)" json:"step_type,omitempty"`
	// QuizScore is the fraction of correct answers in [0, 1] for quiz steps.
	// Nil for non-quiz steps or quizzes that have not been submitted yet.
	QuizScore *float64 `json:"quiz_score,omitempty"`
	// QuizAnswers is the JSON-encoded map of question_id -> submitted answer.
	// Empty for non-quiz steps.
	QuizAnswers string `gorm:"type:text" json:"quiz_answers,omitempty"`
}

// Implement interfaces for entity management system
func (s ScenarioStepProgress) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioStepProgress) GetReferenceObject() string {
	return "ScenarioStepProgress"
}

// TableName specifies the table name
func (ScenarioStepProgress) TableName() string {
	return "scenario_step_progress"
}
