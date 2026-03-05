package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateScenarioStepProgressInput - DTO for creating step progress
type CreateScenarioStepProgressInput struct {
	SessionID uuid.UUID `json:"session_id" mapstructure:"session_id" binding:"required"`
	StepOrder int       `json:"step_order" mapstructure:"step_order" binding:"required"`
	Status    string    `json:"status,omitempty" mapstructure:"status" binding:"omitempty,oneof=locked active completed skipped"`
}

// EditScenarioStepProgressInput - DTO for editing step progress (partial updates)
type EditScenarioStepProgressInput struct {
	Status           *string    `json:"status,omitempty" mapstructure:"status" binding:"omitempty,oneof=locked active completed skipped"`
	VerifyAttempts   *int       `json:"verify_attempts,omitempty" mapstructure:"verify_attempts"`
	CompletedAt      *time.Time `json:"completed_at,omitempty" mapstructure:"completed_at"`
	TimeSpentSeconds *int       `json:"time_spent_seconds,omitempty" mapstructure:"time_spent_seconds"`
}

// ScenarioStepProgressOutput - DTO for step progress responses
type ScenarioStepProgressOutput struct {
	ID               uuid.UUID  `json:"id"`
	SessionID        uuid.UUID  `json:"session_id"`
	StepOrder        int        `json:"step_order"`
	Status           string     `json:"status"`
	VerifyAttempts   int        `json:"verify_attempts"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	TimeSpentSeconds int        `json:"time_spent_seconds"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
