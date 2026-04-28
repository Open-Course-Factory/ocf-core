package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateScenarioStepInput - DTO for creating a new scenario step
type CreateScenarioStepInput struct {
	ScenarioID         uuid.UUID  `json:"scenario_id" mapstructure:"scenario_id" binding:"required"`
	Order              int        `json:"order" mapstructure:"order" binding:"required"`
	Title              string     `json:"title" mapstructure:"title" binding:"required"`
	StepType           string     `json:"step_type,omitempty" mapstructure:"step_type"`
	TextContent        string     `json:"text_content,omitempty" mapstructure:"text_content"`
	HintContent        string     `json:"hint_content,omitempty" mapstructure:"hint_content"`
	VerifyScript       string     `json:"verify_script,omitempty" mapstructure:"verify_script"`
	BackgroundScript   string     `json:"background_script,omitempty" mapstructure:"background_script"`
	ForegroundScript   string     `json:"foreground_script,omitempty" mapstructure:"foreground_script"`
	HasFlag            bool       `json:"has_flag,omitempty" mapstructure:"has_flag"`
	FlagPath           string     `json:"flag_path,omitempty" mapstructure:"flag_path"`
	FlagLevel          int        `json:"flag_level,omitempty" mapstructure:"flag_level"`
	VerifyScriptID     *uuid.UUID `json:"verify_script_id,omitempty" mapstructure:"verify_script_id"`
	BackgroundScriptID *uuid.UUID `json:"background_script_id,omitempty" mapstructure:"background_script_id"`
	ForegroundScriptID *uuid.UUID `json:"foreground_script_id,omitempty" mapstructure:"foreground_script_id"`
	TextFileID         *uuid.UUID `json:"text_file_id,omitempty" mapstructure:"text_file_id"`
	HintFileID         *uuid.UUID `json:"hint_file_id,omitempty" mapstructure:"hint_file_id"`
}

// EditScenarioStepInput - DTO for editing a scenario step (partial updates)
type EditScenarioStepInput struct {
	Order              *int       `json:"order,omitempty" mapstructure:"order"`
	Title              *string    `json:"title,omitempty" mapstructure:"title"`
	StepType           *string    `json:"step_type,omitempty" mapstructure:"step_type"`
	TextContent        *string    `json:"text_content,omitempty" mapstructure:"text_content"`
	HintContent        *string    `json:"hint_content,omitempty" mapstructure:"hint_content"`
	VerifyScript       *string    `json:"verify_script,omitempty" mapstructure:"verify_script"`
	BackgroundScript   *string    `json:"background_script,omitempty" mapstructure:"background_script"`
	ForegroundScript   *string    `json:"foreground_script,omitempty" mapstructure:"foreground_script"`
	HasFlag            *bool      `json:"has_flag,omitempty" mapstructure:"has_flag"`
	FlagPath           *string    `json:"flag_path,omitempty" mapstructure:"flag_path"`
	FlagLevel          *int       `json:"flag_level,omitempty" mapstructure:"flag_level"`
	VerifyScriptID     *uuid.UUID `json:"verify_script_id,omitempty" mapstructure:"verify_script_id"`
	BackgroundScriptID *uuid.UUID `json:"background_script_id,omitempty" mapstructure:"background_script_id"`
	ForegroundScriptID *uuid.UUID `json:"foreground_script_id,omitempty" mapstructure:"foreground_script_id"`
	TextFileID         *uuid.UUID `json:"text_file_id,omitempty" mapstructure:"text_file_id"`
	HintFileID         *uuid.UUID `json:"hint_file_id,omitempty" mapstructure:"hint_file_id"`
}

// ScenarioStepOutput - DTO for scenario step responses (admin-only entity).
// Includes scripts so administrators can view and edit step scripts via the admin panel.
type ScenarioStepOutput struct {
	ID                 uuid.UUID  `json:"id"`
	ScenarioID         uuid.UUID  `json:"scenario_id"`
	Order              int        `json:"order"`
	Title              string     `json:"title"`
	StepType           string     `json:"step_type"`
	TextContent        string     `json:"text_content,omitempty"`
	HintContent        string     `json:"hint_content,omitempty"`
	VerifyScript       string     `json:"verify_script,omitempty"`
	BackgroundScript   string     `json:"background_script,omitempty"`
	ForegroundScript   string     `json:"foreground_script,omitempty"`
	HasFlag            bool       `json:"has_flag"`
	FlagPath           string     `json:"flag_path,omitempty"`
	FlagLevel          int        `json:"flag_level"`
	VerifyScriptID     *uuid.UUID `json:"verify_script_id,omitempty"`
	BackgroundScriptID *uuid.UUID `json:"background_script_id,omitempty"`
	ForegroundScriptID *uuid.UUID `json:"foreground_script_id,omitempty"`
	TextFileID         *uuid.UUID `json:"text_file_id,omitempty"`
	HintFileID         *uuid.UUID `json:"hint_file_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
