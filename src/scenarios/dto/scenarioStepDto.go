package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateScenarioStepInput - DTO for creating a new scenario step
type CreateScenarioStepInput struct {
	ScenarioID       uuid.UUID `json:"scenario_id" mapstructure:"scenario_id" binding:"required"`
	Order            int       `json:"order" mapstructure:"order" binding:"required"`
	Title            string    `json:"title" mapstructure:"title" binding:"required"`
	TextContent      string    `json:"text_content,omitempty" mapstructure:"text_content"`
	HintContent      string    `json:"hint_content,omitempty" mapstructure:"hint_content"`
	VerifyScript     string    `json:"verify_script,omitempty" mapstructure:"verify_script"`
	BackgroundScript string    `json:"background_script,omitempty" mapstructure:"background_script"`
	ForegroundScript string    `json:"foreground_script,omitempty" mapstructure:"foreground_script"`
	HasFlag          bool      `json:"has_flag,omitempty" mapstructure:"has_flag"`
	FlagPath         string    `json:"flag_path,omitempty" mapstructure:"flag_path"`
	FlagLevel        int       `json:"flag_level,omitempty" mapstructure:"flag_level"`
}

// EditScenarioStepInput - DTO for editing a scenario step (partial updates)
type EditScenarioStepInput struct {
	Order            *int    `json:"order,omitempty" mapstructure:"order"`
	Title            *string `json:"title,omitempty" mapstructure:"title"`
	TextContent      *string `json:"text_content,omitempty" mapstructure:"text_content"`
	HintContent      *string `json:"hint_content,omitempty" mapstructure:"hint_content"`
	VerifyScript     *string `json:"verify_script,omitempty" mapstructure:"verify_script"`
	BackgroundScript *string `json:"background_script,omitempty" mapstructure:"background_script"`
	ForegroundScript *string `json:"foreground_script,omitempty" mapstructure:"foreground_script"`
	HasFlag          *bool   `json:"has_flag,omitempty" mapstructure:"has_flag"`
	FlagPath         *string `json:"flag_path,omitempty" mapstructure:"flag_path"`
	FlagLevel        *int    `json:"flag_level,omitempty" mapstructure:"flag_level"`
}

// ScenarioStepOutput - DTO for scenario step responses
// Scripts are intentionally excluded for defense-in-depth: even admin-only
// endpoints should not expose verify/background/foreground scripts in API responses.
type ScenarioStepOutput struct {
	ID          uuid.UUID `json:"id"`
	ScenarioID  uuid.UUID `json:"scenario_id"`
	Order       int       `json:"order"`
	Title       string    `json:"title"`
	TextContent string    `json:"text_content,omitempty"`
	HintContent string    `json:"hint_content,omitempty"`
	HasFlag     bool      `json:"has_flag"`
	FlagPath    string    `json:"flag_path,omitempty"`
	FlagLevel   int       `json:"flag_level"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
