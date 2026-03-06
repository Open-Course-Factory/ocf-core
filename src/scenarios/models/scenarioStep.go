package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// ScenarioStep represents a single step within a scenario
type ScenarioStep struct {
	entityManagementModels.BaseModel
	ScenarioID       uuid.UUID `gorm:"type:uuid;not null;index" json:"scenario_id"`
	Order            int       `gorm:"not null" json:"order"`
	Title            string    `gorm:"type:varchar(500);not null" json:"title"`
	TextContent      string    `gorm:"type:text" json:"text_content,omitempty"`      // markdown
	HintContent      string    `gorm:"type:text" json:"hint_content,omitempty"`      // markdown
	VerifyScript     string    `gorm:"type:text" json:"-"`
	BackgroundScript string    `gorm:"type:text" json:"-"`
	ForegroundScript string    `gorm:"type:text" json:"-"`
	HasFlag          bool      `gorm:"default:false" json:"has_flag"`
	FlagLevel        int       `gorm:"default:0" json:"flag_level"`
}

// Implement interfaces for entity management system
func (s ScenarioStep) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioStep) GetReferenceObject() string {
	return "ScenarioStep"
}

// TableName specifies the table name
func (ScenarioStep) TableName() string {
	return "scenario_steps"
}
