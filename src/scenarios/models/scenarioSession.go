package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// ScenarioSession represents a user's active session working through a scenario
type ScenarioSession struct {
	entityManagementModels.BaseModel
	ScenarioID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"scenario_id"`
	UserID            string     `gorm:"type:varchar(255);not null;index" json:"user_id"`
	TerminalSessionID *string    `gorm:"type:varchar(255)" json:"terminal_session_id,omitempty"`
	CurrentStep       int        `gorm:"default:0" json:"current_step"`
	Status            string     `gorm:"type:varchar(50);default:'active'" json:"status"` // active, completed, abandoned
	StartedAt         time.Time  `gorm:"not null" json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`

	// Relations
	StepProgress []ScenarioStepProgress `gorm:"foreignKey:SessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"step_progress,omitempty"`
	Flags        []ScenarioFlag         `gorm:"foreignKey:SessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"flags,omitempty"`
	Scenario     Scenario               `gorm:"foreignKey:ScenarioID" json:"-"`
}

// Implement interfaces for entity management system
func (s ScenarioSession) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioSession) GetReferenceObject() string {
	return "ScenarioSession"
}

// TableName specifies the table name
func (ScenarioSession) TableName() string {
	return "scenario_sessions"
}
