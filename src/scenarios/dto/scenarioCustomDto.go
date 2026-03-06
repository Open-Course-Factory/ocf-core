package dto

import (
	"time"

	"github.com/google/uuid"
)

// SessionResponse - DTO for scenario session information
type SessionResponse struct {
	ID                string    `json:"id"`
	ScenarioID        string    `json:"scenario_id"`
	UserID            string    `json:"user_id"`
	TerminalSessionID string    `json:"terminal_session_id"`
	CurrentStep       int       `json:"current_step"`
	Status            string    `json:"status"`
	Grade             *float64  `json:"grade,omitempty"`
	StartedAt         time.Time `json:"started_at"`
}

// MessageResponse - DTO for simple message responses
type MessageResponse struct {
	Message string `json:"message"`
}

// ImportScenarioInput - DTO for importing a scenario from a git repository
type ImportScenarioInput struct {
	GitRepository string `json:"git_repository" binding:"required"`
	GitBranch     string `json:"git_branch,omitempty"` // defaults to "main"
	SourcePath    string `json:"source_path,omitempty"`
}

// StartScenarioInput - DTO for starting a scenario session
type StartScenarioInput struct {
	ScenarioID        string `json:"scenario_id" binding:"required"`
	TerminalSessionID string `json:"terminal_session_id" binding:"required"`
	Backend           string `json:"backend,omitempty"`
	InstanceType      string `json:"instance_type,omitempty"`
}

// VerifyStepResponse - DTO for verify step results
type VerifyStepResponse struct {
	Passed   bool   `json:"passed"`
	Output   string `json:"output,omitempty"`
	NextStep *int   `json:"next_step,omitempty"`
}

// SubmitFlagInput - DTO for submitting a flag answer
type SubmitFlagInput struct {
	Flag string `json:"flag" binding:"required"`
}

// SubmitFlagResponse - DTO for flag submission results
type SubmitFlagResponse struct {
	Correct  bool   `json:"correct"`
	Message  string `json:"message,omitempty"`
	NextStep *int   `json:"next_step,omitempty"`
}

// CurrentStepResponse - DTO for current step information
type CurrentStepResponse struct {
	StepOrder  int    `json:"step_order"`
	TotalSteps int    `json:"total_steps"`
	Title      string `json:"title"`
	Text       string `json:"text,omitempty"`
	Hint       string `json:"hint,omitempty"`
	Status     string `json:"status"`
	HasFlag    bool   `json:"has_flag"`
}

// SeedScenarioInput - DTO for seeding a scenario with inline content (admin/testing)
type SeedScenarioInput struct {
	Title         string          `json:"title" binding:"required"`
	Description   string          `json:"description"`
	Difficulty    string          `json:"difficulty"`
	EstimatedTime string          `json:"estimated_time"`
	InstanceType  string          `json:"instance_type"`
	OsType        string          `json:"os_type"`
	FlagsEnabled  bool            `json:"flags_enabled"`
	GshEnabled    bool            `json:"gsh_enabled"`
	CrashTraps    bool            `json:"crash_traps"`
	IntroText     string          `json:"intro_text"`
	FinishText    string          `json:"finish_text"`
	Steps         []SeedStepInput `json:"steps" binding:"required,min=1"`
}

// SeedStepInput - DTO for a single step in a seed scenario
type SeedStepInput struct {
	Title            string `json:"title" binding:"required"`
	TextContent      string `json:"text_content"`
	HintContent      string `json:"hint_content"`
	VerifyScript     string `json:"verify_script"`
	BackgroundScript string `json:"background_script"`
	ForegroundScript string `json:"foreground_script"`
	HasFlag          bool   `json:"has_flag"`
	FlagPath         string `json:"flag_path"`
}

// MySessionResponse - DTO for a learner's own scenario session
type MySessionResponse struct {
	ID                uuid.UUID  `json:"id"`
	ScenarioID        uuid.UUID  `json:"scenario_id"`
	ScenarioTitle     string     `json:"scenario_title"`
	Status            string     `json:"status"`
	Grade             *float64   `json:"grade,omitempty"`
	CurrentStep       int        `json:"current_step"`
	TotalSteps        int        `json:"total_steps"`
	CompletedSteps    int        `json:"completed_steps"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	TerminalSessionID *string    `json:"terminal_session_id,omitempty"`
}
