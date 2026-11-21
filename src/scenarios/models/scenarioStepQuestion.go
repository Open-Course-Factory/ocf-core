package models

import (
	"encoding/json"

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
	// OptionPoints gives each option of a multiple_choice question its own
	// points (JSON array of non-negative ints, one per option); empty means
	// right-or-wrong. Hidden like CorrectAnswer: the best option is the
	// answer. See services.ScoreQuiz.
	OptionPoints string `gorm:"type:text" json:"-"`
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

// ParsedOptionPoints decodes OptionPoints. It returns nil, nil when the
// question has none, and an error when the column is not a JSON array of
// integers. Validation reports that error; scoring treats it as "none".
func (s ScenarioStepQuestion) ParsedOptionPoints() ([]int, error) {
	if s.OptionPoints == "" {
		return nil, nil
	}
	var points []int
	if err := json.Unmarshal([]byte(s.OptionPoints), &points); err != nil {
		return nil, err
	}
	return points, nil
}
