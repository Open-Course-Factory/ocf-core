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
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	TimeSpentSeconds int        `gorm:"default:0" json:"time_spent_seconds"`
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
