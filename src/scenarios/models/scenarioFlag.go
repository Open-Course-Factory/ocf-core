package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// ScenarioFlag tracks flag submissions for CTF-style challenges within a session
type ScenarioFlag struct {
	entityManagementModels.BaseModel
	SessionID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"session_id"`
	StepOrder     int        `gorm:"not null" json:"step_order"`
	ExpectedFlag  string     `gorm:"type:varchar(500)" json:"-"` // never exposed in API
	SubmittedFlag *string    `gorm:"type:varchar(500)" json:"submitted_flag,omitempty"`
	SubmittedAt   *time.Time `json:"submitted_at,omitempty"`
	IsCorrect     bool       `gorm:"default:false" json:"is_correct"`
}

// Implement interfaces for entity management system
func (f ScenarioFlag) GetBaseModel() entityManagementModels.BaseModel {
	return f.BaseModel
}

func (f ScenarioFlag) GetReferenceObject() string {
	return "ScenarioFlag"
}

// TableName specifies the table name
func (ScenarioFlag) TableName() string {
	return "scenario_flags"
}
