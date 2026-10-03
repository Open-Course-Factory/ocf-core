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
	ShowImmediateFeedback bool    `json:"show_immediate_feedback,omitempty" mapstructure:"show_immediate_feedback"`
	TextContent        string     `json:"text_content,omitempty" mapstructure:"text_content"`
	HintContent        string     `json:"hint_content,omitempty" mapstructure:"hint_content"`
	VerifyScript       string     `json:"verify_script,omitempty" mapstructure:"verify_script"`
	BackgroundScript   string     `json:"background_script,omitempty" mapstructure:"background_script"`
	ForegroundScript   string     `json:"foreground_script,omitempty" mapstructure:"foreground_script"`
	IntroEffect        string     `json:"intro_effect,omitempty" mapstructure:"intro_effect"`
	IntroText          string     `json:"intro_text,omitempty" mapstructure:"intro_text" binding:"max=500"`
	OutroEffect        string     `json:"outro_effect,omitempty" mapstructure:"outro_effect"`
	OutroText          string     `json:"outro_text,omitempty" mapstructure:"outro_text" binding:"max=500"`
	BackgroundTimeoutSeconds int  `json:"background_timeout_seconds,omitempty" mapstructure:"background_timeout_seconds"`
	BackgroundAsync    bool       `json:"background_async,omitempty" mapstructure:"background_async"`
	HasFlag            bool       `json:"has_flag,omitempty" mapstructure:"has_flag"`
	FlagPath           string     `json:"flag_path,omitempty" mapstructure:"flag_path"`
	FlagLevel          int        `json:"flag_level,omitempty" mapstructure:"flag_level"`
}

// EditScenarioStepInput - DTO for editing a scenario step (partial updates)
type EditScenarioStepInput struct {
	Order              *int       `json:"order,omitempty" mapstructure:"order"`
	Title              *string    `json:"title,omitempty" mapstructure:"title"`
	StepType           *string    `json:"step_type,omitempty" mapstructure:"step_type"`
	ShowImmediateFeedback *bool   `json:"show_immediate_feedback,omitempty" mapstructure:"show_immediate_feedback"`
	TextContent        *string    `json:"text_content,omitempty" mapstructure:"text_content"`
	HintContent        *string    `json:"hint_content,omitempty" mapstructure:"hint_content"`
	VerifyScript       *string    `json:"verify_script,omitempty" mapstructure:"verify_script"`
	BackgroundScript   *string    `json:"background_script,omitempty" mapstructure:"background_script"`
	ForegroundScript   *string    `json:"foreground_script,omitempty" mapstructure:"foreground_script"`
	IntroEffect        *string    `json:"intro_effect,omitempty" mapstructure:"intro_effect"`
	IntroText          *string    `json:"intro_text,omitempty" mapstructure:"intro_text" binding:"omitempty,max=500"`
	OutroEffect        *string    `json:"outro_effect,omitempty" mapstructure:"outro_effect"`
	OutroText          *string    `json:"outro_text,omitempty" mapstructure:"outro_text" binding:"omitempty,max=500"`
	BackgroundTimeoutSeconds *int `json:"background_timeout_seconds,omitempty" mapstructure:"background_timeout_seconds"`
	BackgroundAsync    *bool      `json:"background_async,omitempty" mapstructure:"background_async"`
	HasFlag            *bool      `json:"has_flag,omitempty" mapstructure:"has_flag"`
	FlagPath           *string    `json:"flag_path,omitempty" mapstructure:"flag_path"`
	FlagLevel          *int       `json:"flag_level,omitempty" mapstructure:"flag_level"`
}

// ScenarioStepOutput - DTO for scenario step responses (admin-only entity).
// Includes scripts so administrators can view and edit step scripts via the admin panel.
type ScenarioStepOutput struct {
	ID                 uuid.UUID  `json:"id"`
	ScenarioID         uuid.UUID  `json:"scenario_id"`
	Order              int        `json:"order"`
	Title              string     `json:"title"`
	StepType           string     `json:"step_type"`
	ShowImmediateFeedback bool    `json:"show_immediate_feedback"`
	TextContent        string     `json:"text_content,omitempty"`
	HintContent        string     `json:"hint_content,omitempty"`
	VerifyScript       string     `json:"verify_script,omitempty"`
	BackgroundScript   string     `json:"background_script,omitempty"`
	ForegroundScript   string     `json:"foreground_script,omitempty"`
	IntroEffect        string     `json:"intro_effect,omitempty"`
	IntroText          string     `json:"intro_text,omitempty"`
	OutroEffect        string     `json:"outro_effect,omitempty"`
	OutroText          string     `json:"outro_text,omitempty"`
	BackgroundTimeoutSeconds int  `json:"background_timeout_seconds,omitempty"`
	BackgroundAsync    bool       `json:"background_async,omitempty"`
	HasFlag            bool       `json:"has_flag"`
	FlagPath           string     `json:"flag_path,omitempty"`
	FlagLevel          int        `json:"flag_level"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	Questions          []ScenarioStepQuestionOutput `json:"questions,omitempty"`
}
