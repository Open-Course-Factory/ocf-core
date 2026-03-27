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
	HasFlag          bool               `gorm:"default:false" json:"has_flag"`
	FlagPath         string             `gorm:"type:varchar(500)" json:"flag_path,omitempty"` // where to place the flag file in the container
	FlagLevel        int                `gorm:"default:0" json:"flag_level"`
	VerifyScriptID     *uuid.UUID `gorm:"type:uuid;index" json:"verify_script_id,omitempty" mapstructure:"verify_script_id"`
	BackgroundScriptID *uuid.UUID `gorm:"type:uuid;index" json:"background_script_id,omitempty" mapstructure:"background_script_id"`
	ForegroundScriptID *uuid.UUID `gorm:"type:uuid;index" json:"foreground_script_id,omitempty" mapstructure:"foreground_script_id"`
	TextFileID         *uuid.UUID `gorm:"type:uuid;index" json:"text_file_id,omitempty" mapstructure:"text_file_id"`
	HintFileID         *uuid.UUID `gorm:"type:uuid;index" json:"hint_file_id,omitempty" mapstructure:"hint_file_id"`
	Hints            []ScenarioStepHint `gorm:"foreignKey:StepID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"hints,omitempty"`
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
