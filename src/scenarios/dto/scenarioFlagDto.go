package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateScenarioFlagInput - DTO for creating a flag entry
type CreateScenarioFlagInput struct {
	SessionID    uuid.UUID `json:"session_id" mapstructure:"session_id" binding:"required"`
	StepOrder    int       `json:"step_order" mapstructure:"step_order" binding:"required"`
	ExpectedFlag string    `json:"expected_flag" mapstructure:"expected_flag" binding:"required"`
}

// EditScenarioFlagInput - DTO for editing a flag entry (partial updates)
type EditScenarioFlagInput struct {
	SubmittedFlag *string    `json:"submitted_flag,omitempty" mapstructure:"submitted_flag"`
	SubmittedAt   *time.Time `json:"submitted_at,omitempty" mapstructure:"submitted_at"`
	IsCorrect     *bool      `json:"is_correct,omitempty" mapstructure:"is_correct"`
}

// ScenarioFlagOutput - DTO for flag responses (never exposes expected_flag)
type ScenarioFlagOutput struct {
	ID            uuid.UUID  `json:"id"`
	SessionID     uuid.UUID  `json:"session_id"`
	StepOrder     int        `json:"step_order"`
	SubmittedFlag *string    `json:"submitted_flag,omitempty"`
	SubmittedAt   *time.Time `json:"submitted_at,omitempty"`
	IsCorrect     bool       `json:"is_correct"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
