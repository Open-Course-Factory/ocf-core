package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateScenarioStepHintInput - DTO for creating a new scenario step hint
type CreateScenarioStepHintInput struct {
	StepID  uuid.UUID `json:"step_id" mapstructure:"step_id" binding:"required"`
	Level   int       `json:"level" mapstructure:"level" binding:"required,min=1,max=5"`
	Content string    `json:"content" mapstructure:"content" binding:"required"`
}

// EditScenarioStepHintInput - DTO for editing a scenario step hint (partial updates)
type EditScenarioStepHintInput struct {
	Level   *int    `json:"level,omitempty" mapstructure:"level"`
	Content *string `json:"content,omitempty" mapstructure:"content"`
}

// ScenarioStepHintOutput - DTO for scenario step hint responses
type ScenarioStepHintOutput struct {
	ID        uuid.UUID `json:"id"`
	StepID    uuid.UUID `json:"step_id"`
	Level     int       `json:"level"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
