package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// ScenarioStepHint represents a single progressive hint for a scenario step
type ScenarioStepHint struct {
	entityManagementModels.BaseModel
	StepID  uuid.UUID `gorm:"type:uuid;not null;index" json:"step_id"`
	Level   int       `gorm:"not null" json:"level"`
	Content string    `gorm:"type:text;not null" json:"content"`
}

func (s ScenarioStepHint) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioStepHint) GetReferenceObject() string {
	return "ScenarioStepHint"
}

func (ScenarioStepHint) TableName() string {
	return "scenario_step_hints"
}
