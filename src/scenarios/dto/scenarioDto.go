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
	IntroText      string     `json:"intro_text,omitempty" mapstructure:"intro_text"`
	FinishText     string     `json:"finish_text,omitempty" mapstructure:"finish_text"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty" mapstructure:"organization_id"`
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
	IntroText      *string    `json:"intro_text,omitempty" mapstructure:"intro_text"`
	FinishText     *string    `json:"finish_text,omitempty" mapstructure:"finish_text"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty" mapstructure:"organization_id"`
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
	IntroText      string             `json:"intro_text,omitempty"`
	FinishText     string             `json:"finish_text,omitempty"`
	CreatedByID    string             `json:"created_by_id"`
	OrganizationID *uuid.UUID         `json:"organization_id,omitempty"`
	SetupScriptID  *uuid.UUID         `json:"setup_script_id,omitempty"`
	IntroFileID    *uuid.UUID         `json:"intro_file_id,omitempty"`
	FinishFileID   *uuid.UUID         `json:"finish_file_id,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Steps          []ScenarioStepOutput `json:"steps,omitempty"`
}
