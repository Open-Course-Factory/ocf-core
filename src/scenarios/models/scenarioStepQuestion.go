package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// ScenarioStepQuestion represents a quiz question within a scenario step
type ScenarioStepQuestion struct {
	entityManagementModels.BaseModel
	StepID        uuid.UUID `gorm:"type:uuid;not null;index" json:"step_id"`
	Order         int       `gorm:"not null" json:"order"`
	QuestionText  string    `gorm:"type:text;not null" json:"question_text"`
	QuestionType  string    `gorm:"type:varchar(50);not null" json:"question_type"` // multiple_choice, free_text, true_false
	Options       string    `gorm:"type:text" json:"options,omitempty"`             // JSON array for multiple choice
	CorrectAnswer string    `gorm:"type:text" json:"-"`                             // hidden from API
	Explanation   string    `gorm:"type:text" json:"explanation,omitempty"`
	Points        int       `gorm:"default:1" json:"points"`
}

func (s ScenarioStepQuestion) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioStepQuestion) GetReferenceObject() string {
	return "ScenarioStepQuestion"
}

func (ScenarioStepQuestion) TableName() string {
	return "scenario_step_questions"
}
