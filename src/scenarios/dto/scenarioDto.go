package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateScenarioInput - DTO for creating a new scenario
type CreateScenarioInput struct {
	Name           string     `json:"name" mapstructure:"name" binding:"required"`
	Title          string     `json:"title" mapstructure:"title" binding:"required"`
	Description    string     `json:"description,omitempty" mapstructure:"description"`
	Difficulty     string     `json:"difficulty,omitempty" mapstructure:"difficulty" binding:"omitempty,oneof=beginner intermediate advanced"`
	EstimatedTime  string     `json:"estimated_time,omitempty" mapstructure:"estimated_time"`
	InstanceType   string     `json:"instance_type" mapstructure:"instance_type" binding:"required"`
	Hostname       string     `json:"hostname,omitempty" mapstructure:"hostname" binding:"omitempty,max=63"`
	OsType         string     `json:"os_type,omitempty" mapstructure:"os_type" binding:"omitempty,oneof=deb rpm apk pacman"`
	SourceType     string     `json:"source_type,omitempty" mapstructure:"source_type" binding:"omitempty,oneof=git upload builtin"`
	GitRepository  string     `json:"git_repository,omitempty" mapstructure:"git_repository"`
	GitBranch      string     `json:"git_branch,omitempty" mapstructure:"git_branch"`
	SourcePath     string     `json:"source_path,omitempty" mapstructure:"source_path"`
	FlagsEnabled   bool       `json:"flags_enabled,omitempty" mapstructure:"flags_enabled"`
	GshEnabled     bool       `json:"gsh_enabled,omitempty" mapstructure:"gsh_enabled"`
	CrashTraps     bool       `json:"crash_traps,omitempty" mapstructure:"crash_traps"`
	Objectives     string     `json:"objectives,omitempty" mapstructure:"objectives" binding:"omitempty,max=5000"`
	Prerequisites  string     `json:"prerequisites,omitempty" mapstructure:"prerequisites" binding:"omitempty,max=5000"`
	IntroText      string     `json:"intro_text,omitempty" mapstructure:"intro_text"`
	FinishText     string     `json:"finish_text,omitempty" mapstructure:"finish_text"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty" mapstructure:"organization_id"`
	IsPublic       bool       `json:"is_public,omitempty" mapstructure:"is_public"`
	SetupScriptID  *uuid.UUID `json:"setup_script_id,omitempty" mapstructure:"setup_script_id"`
	IntroFileID    *uuid.UUID `json:"intro_file_id,omitempty" mapstructure:"intro_file_id"`
	FinishFileID   *uuid.UUID `json:"finish_file_id,omitempty" mapstructure:"finish_file_id"`
}

// EditScenarioInput - DTO for editing a scenario (partial updates)
// All fields are pointers to support PATCH semantics
type EditScenarioInput struct {
	Name           *string    `json:"name,omitempty" mapstructure:"name"`
	Title          *string    `json:"title,omitempty" mapstructure:"title"`
	Description    *string    `json:"description,omitempty" mapstructure:"description"`
	Difficulty     *string    `json:"difficulty,omitempty" mapstructure:"difficulty" binding:"omitempty,oneof=beginner intermediate advanced"`
	EstimatedTime  *string    `json:"estimated_time,omitempty" mapstructure:"estimated_time"`
	InstanceType   *string    `json:"instance_type,omitempty" mapstructure:"instance_type"`
	Hostname       *string    `json:"hostname,omitempty" mapstructure:"hostname" binding:"omitempty,max=63"`
	OsType         *string    `json:"os_type,omitempty" mapstructure:"os_type" binding:"omitempty,oneof=deb rpm apk pacman"`
	SourceType     *string    `json:"source_type,omitempty" mapstructure:"source_type" binding:"omitempty,oneof=git upload builtin"`
	GitRepository  *string    `json:"git_repository,omitempty" mapstructure:"git_repository"`
	GitBranch      *string    `json:"git_branch,omitempty" mapstructure:"git_branch"`
	SourcePath     *string    `json:"source_path,omitempty" mapstructure:"source_path"`
	FlagsEnabled   *bool      `json:"flags_enabled,omitempty" mapstructure:"flags_enabled"`
	GshEnabled     *bool      `json:"gsh_enabled,omitempty" mapstructure:"gsh_enabled"`
	CrashTraps     *bool      `json:"crash_traps,omitempty" mapstructure:"crash_traps"`
	Objectives     *string    `json:"objectives,omitempty" mapstructure:"objectives" binding:"omitempty,max=5000"`
	Prerequisites  *string    `json:"prerequisites,omitempty" mapstructure:"prerequisites" binding:"omitempty,max=5000"`
	IntroText      *string    `json:"intro_text,omitempty" mapstructure:"intro_text"`
	FinishText     *string    `json:"finish_text,omitempty" mapstructure:"finish_text"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty" mapstructure:"organization_id"`
	IsPublic       *bool      `json:"is_public,omitempty" mapstructure:"is_public"`
	SetupScriptID  *uuid.UUID `json:"setup_script_id,omitempty" mapstructure:"setup_script_id"`
	IntroFileID    *uuid.UUID `json:"intro_file_id,omitempty" mapstructure:"intro_file_id"`
	FinishFileID   *uuid.UUID `json:"finish_file_id,omitempty" mapstructure:"finish_file_id"`
}

// ScenarioOutput - DTO for scenario responses
type ScenarioOutput struct {
	ID             uuid.UUID          `json:"id"`
	Name           string             `json:"name"`
	Title          string             `json:"title"`
	Description    string             `json:"description,omitempty"`
	Difficulty     string             `json:"difficulty"`
	EstimatedTime  string             `json:"estimated_time"`
	InstanceType   string             `json:"instance_type"`
	Hostname       string             `json:"hostname,omitempty"`
	OsType         string             `json:"os_type,omitempty"`
	SourceType     string             `json:"source_type"`
	GitRepository  string             `json:"git_repository,omitempty"`
	GitBranch      string             `json:"git_branch"`
	SourcePath     string             `json:"source_path,omitempty"`
	FlagsEnabled   bool               `json:"flags_enabled"`
	GshEnabled     bool               `json:"gsh_enabled"`
	CrashTraps     bool               `json:"crash_traps"`
	Objectives     string             `json:"objectives,omitempty"`
	Prerequisites  string             `json:"prerequisites,omitempty"`
	IntroText      string             `json:"intro_text,omitempty"`
	FinishText     string             `json:"finish_text,omitempty"`
	CreatedByID    string             `json:"created_by_id"`
	OrganizationID *uuid.UUID         `json:"organization_id,omitempty"`
	IsPublic       bool               `json:"is_public"`
	SetupScriptID  *uuid.UUID         `json:"setup_script_id,omitempty"`
	IntroFileID    *uuid.UUID         `json:"intro_file_id,omitempty"`
	FinishFileID   *uuid.UUID         `json:"finish_file_id,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Steps                  []ScenarioStepOutput          `json:"steps,omitempty"`
	CompatibleInstanceTypes []ScenarioInstanceTypeOutput `json:"compatible_instance_types,omitempty"`
}

// ScenarioInstanceType DTOs
type CreateScenarioInstanceTypeInput struct {
	ScenarioID   uuid.UUID `json:"scenario_id" mapstructure:"scenario_id" binding:"required"`
	InstanceType string    `json:"instance_type" mapstructure:"instance_type" binding:"required"`
	OsType       string    `json:"os_type,omitempty" mapstructure:"os_type"`
	Priority     int       `json:"priority,omitempty" mapstructure:"priority"`
}

type EditScenarioInstanceTypeInput struct {
	InstanceType *string `json:"instance_type,omitempty" mapstructure:"instance_type"`
	OsType       *string `json:"os_type,omitempty" mapstructure:"os_type"`
	Priority     *int    `json:"priority,omitempty" mapstructure:"priority"`
}

type ScenarioInstanceTypeOutput struct {
	ID           uuid.UUID `json:"id"`
	ScenarioID   uuid.UUID `json:"scenario_id"`
	InstanceType string    `json:"instance_type"`
	OsType       string    `json:"os_type,omitempty"`
	Priority     int       `json:"priority"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
