package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateScenarioSessionInput - DTO for creating a new scenario session
type CreateScenarioSessionInput struct {
	ScenarioID        uuid.UUID `json:"scenario_id" mapstructure:"scenario_id" binding:"required"`
	UserID            string    `json:"user_id" mapstructure:"user_id" binding:"required"`
	TerminalSessionID *string   `json:"terminal_session_id,omitempty" mapstructure:"terminal_session_id"`
}

// EditScenarioSessionInput - DTO for editing a scenario session (partial updates)
type EditScenarioSessionInput struct {
	TerminalSessionID *string    `json:"terminal_session_id,omitempty" mapstructure:"terminal_session_id"`
	CurrentStep       *int       `json:"current_step,omitempty" mapstructure:"current_step"`
	Status            *string    `json:"status,omitempty" mapstructure:"status" binding:"omitempty,oneof=active completed abandoned"`
	CompletedAt       *time.Time `json:"completed_at,omitempty" mapstructure:"completed_at"`
}

// ScenarioSessionOutput - DTO for scenario session responses
type ScenarioSessionOutput struct {
	ID                uuid.UUID                    `json:"id"`
	ScenarioID        uuid.UUID                    `json:"scenario_id"`
	UserID            string                       `json:"user_id"`
	TerminalSessionID *string                      `json:"terminal_session_id,omitempty"`
	CurrentStep       int                          `json:"current_step"`
	Status            string                       `json:"status"`
	StartedAt         time.Time                    `json:"started_at"`
	CompletedAt       *time.Time                   `json:"completed_at,omitempty"`
	CreatedAt         time.Time                    `json:"created_at"`
	UpdatedAt         time.Time                    `json:"updated_at"`
	StepProgress      []ScenarioStepProgressOutput `json:"step_progress,omitempty"`
	Flags             []ScenarioFlagOutput         `json:"flags,omitempty"`
}
