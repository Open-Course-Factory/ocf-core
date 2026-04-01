package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateScenarioAssignmentInput - DTO for creating a new scenario assignment
type CreateScenarioAssignmentInput struct {
	ScenarioID     uuid.UUID  `json:"scenario_id" mapstructure:"scenario_id" binding:"required"`
	GroupID        *uuid.UUID `json:"group_id,omitempty" mapstructure:"group_id"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty" mapstructure:"organization_id"`
	Scope          string     `json:"scope" mapstructure:"scope" binding:"required,oneof=group org"`
	StartDate      *time.Time `json:"start_date,omitempty" mapstructure:"start_date"`
	Deadline       *time.Time `json:"deadline,omitempty" mapstructure:"deadline"`
	IsActive       bool       `json:"is_active,omitempty" mapstructure:"is_active"`
}

// EditScenarioAssignmentInput - DTO for editing a scenario assignment (partial updates)
type EditScenarioAssignmentInput struct {
	StartDate *time.Time `json:"start_date,omitempty" mapstructure:"start_date"`
	Deadline  *time.Time `json:"deadline,omitempty" mapstructure:"deadline"`
	IsActive  *bool      `json:"is_active,omitempty" mapstructure:"is_active"`
}

// ScenarioAssignmentOutput - DTO for scenario assignment responses
type ScenarioAssignmentOutput struct {
	ID             uuid.UUID       `json:"id"`
	ScenarioID     uuid.UUID       `json:"scenario_id"`
	GroupID        *uuid.UUID      `json:"group_id,omitempty"`
	OrganizationID *uuid.UUID      `json:"organization_id,omitempty"`
	Scope          string          `json:"scope"`
	CreatedByID    string          `json:"created_by_id"`
	StartDate      *time.Time      `json:"start_date,omitempty"`
	Deadline       *time.Time      `json:"deadline,omitempty"`
	IsActive       bool            `json:"is_active"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Scenario       *ScenarioOutput `json:"scenario,omitempty"`
}
