package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateScenarioStepQuestionInput - DTO for creating a new scenario step question
type CreateScenarioStepQuestionInput struct {
	StepID        uuid.UUID `json:"step_id" mapstructure:"step_id" binding:"required"`
	Order         int       `json:"order" mapstructure:"order" binding:"required"`
	QuestionText  string    `json:"question_text" mapstructure:"question_text" binding:"required"`
	QuestionType  string    `json:"question_type" mapstructure:"question_type" binding:"required"`
	Options       string    `json:"options,omitempty" mapstructure:"options"`
	CorrectAnswer string    `json:"correct_answer,omitempty" mapstructure:"correct_answer"`
	Explanation   string    `json:"explanation,omitempty" mapstructure:"explanation"`
	Points        int       `json:"points,omitempty" mapstructure:"points"`
}

// EditScenarioStepQuestionInput - DTO for editing a scenario step question (partial updates)
type EditScenarioStepQuestionInput struct {
	Order         *int    `json:"order,omitempty" mapstructure:"order"`
	QuestionText  *string `json:"question_text,omitempty" mapstructure:"question_text"`
	QuestionType  *string `json:"question_type,omitempty" mapstructure:"question_type"`
	Options       *string `json:"options,omitempty" mapstructure:"options"`
	CorrectAnswer *string `json:"correct_answer,omitempty" mapstructure:"correct_answer"`
	Explanation   *string `json:"explanation,omitempty" mapstructure:"explanation"`
	Points        *int    `json:"points,omitempty" mapstructure:"points"`
}

// ScenarioStepQuestionOutput - DTO for scenario step question responses (admin-only entity).
// Includes correct_answer so administrators can manage quiz answers via the admin panel.
type ScenarioStepQuestionOutput struct {
	ID            uuid.UUID `json:"id"`
	StepID        uuid.UUID `json:"step_id"`
	Order         int       `json:"order"`
	QuestionText  string    `json:"question_text"`
	QuestionType  string    `json:"question_type"`
	Options       string    `json:"options,omitempty"`
	CorrectAnswer string    `json:"correct_answer,omitempty"`
	Explanation   string    `json:"explanation,omitempty"`
	Points        int       `json:"points"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
