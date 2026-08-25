package dto

import (
	"time"

	"github.com/google/uuid"
)

// Translation DTOs deliberately expose no source_hash on input.
//
// The hash records which version of a step a translation was written against,
// and it is the only thing that can say a translation has fallen behind. A
// caller able to set it is a caller able to declare stale work current, so the
// server stamps it and the input has no field for it at all — an omission that
// cannot be worked around rather than one enforced by a check.

// CreateScenarioStepTranslationInput - DTO for translating a scenario step
type CreateScenarioStepTranslationInput struct {
	StepID      uuid.UUID `json:"step_id" mapstructure:"step_id" binding:"required"`
	Locale      string    `json:"locale" mapstructure:"locale" binding:"required"`
	Title       string    `json:"title,omitempty" mapstructure:"title"`
	TextContent string    `json:"text_content,omitempty" mapstructure:"text_content"`
	HintContent string    `json:"hint_content,omitempty" mapstructure:"hint_content"`
	IntroText   string    `json:"intro_text,omitempty" mapstructure:"intro_text"`
	OutroText   string    `json:"outro_text,omitempty" mapstructure:"outro_text"`
}

// EditScenarioStepTranslationInput - partial update of a step translation
type EditScenarioStepTranslationInput struct {
	Title       *string `json:"title,omitempty" mapstructure:"title"`
	TextContent *string `json:"text_content,omitempty" mapstructure:"text_content"`
	HintContent *string `json:"hint_content,omitempty" mapstructure:"hint_content"`
	IntroText   *string `json:"intro_text,omitempty" mapstructure:"intro_text"`
	OutroText   *string `json:"outro_text,omitempty" mapstructure:"outro_text"`
}

// ScenarioStepTranslationOutput - DTO for step translation responses
type ScenarioStepTranslationOutput struct {
	ID          uuid.UUID `json:"id"`
	StepID      uuid.UUID `json:"step_id"`
	Locale      string    `json:"locale"`
	Title       string    `json:"title,omitempty"`
	TextContent string    `json:"text_content,omitempty"`
	HintContent string    `json:"hint_content,omitempty"`
	IntroText   string    `json:"intro_text,omitempty"`
	OutroText   string    `json:"outro_text,omitempty"`
	// SourceHash is reported so an editor can show which translations have
	// fallen behind without asking a second endpoint.
	SourceHash string    `json:"source_hash,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateScenarioTranslationInput - DTO for translating a scenario's own fields
type CreateScenarioTranslationInput struct {
	ScenarioID    uuid.UUID `json:"scenario_id" mapstructure:"scenario_id" binding:"required"`
	Locale        string    `json:"locale" mapstructure:"locale" binding:"required"`
	Title         string    `json:"title,omitempty" mapstructure:"title"`
	Description   string    `json:"description,omitempty" mapstructure:"description"`
	Objectives    string    `json:"objectives,omitempty" mapstructure:"objectives"`
	Prerequisites string    `json:"prerequisites,omitempty" mapstructure:"prerequisites"`
	IntroText     string    `json:"intro_text,omitempty" mapstructure:"intro_text"`
	FinishText    string    `json:"finish_text,omitempty" mapstructure:"finish_text"`
}

// EditScenarioTranslationInput - partial update of a scenario translation
type EditScenarioTranslationInput struct {
	Title         *string `json:"title,omitempty" mapstructure:"title"`
	Description   *string `json:"description,omitempty" mapstructure:"description"`
	Objectives    *string `json:"objectives,omitempty" mapstructure:"objectives"`
	Prerequisites *string `json:"prerequisites,omitempty" mapstructure:"prerequisites"`
	IntroText     *string `json:"intro_text,omitempty" mapstructure:"intro_text"`
	FinishText    *string `json:"finish_text,omitempty" mapstructure:"finish_text"`
}

// ScenarioTranslationOutput - DTO for scenario translation responses
type ScenarioTranslationOutput struct {
	ID            uuid.UUID `json:"id"`
	ScenarioID    uuid.UUID `json:"scenario_id"`
	Locale        string    `json:"locale"`
	Title         string    `json:"title,omitempty"`
	Description   string    `json:"description,omitempty"`
	Objectives    string    `json:"objectives,omitempty"`
	Prerequisites string    `json:"prerequisites,omitempty"`
	IntroText     string    `json:"intro_text,omitempty"`
	FinishText    string    `json:"finish_text,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
