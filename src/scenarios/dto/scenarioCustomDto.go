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
	ProvisioningPhase string    `json:"provisioning_phase,omitempty"`
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
	StepOrder       int    `json:"step_order"`
	TotalSteps      int    `json:"total_steps"`
	Title           string `json:"title"`
	Text            string `json:"text,omitempty"`
	Hint            string `json:"hint,omitempty"`
	HintsTotalCount int    `json:"hints_total_count"`
	HintsRevealed   int    `json:"hints_revealed"`
	Status          string `json:"status"`
	HasFlag         bool   `json:"has_flag"`
}

// RevealHintResponse - DTO for revealing a progressive hint
type RevealHintResponse struct {
	Level   int    `json:"level"`
	Content string `json:"content"`
	Total   int    `json:"total"`
}

// SeedScenarioInput - DTO for seeding a scenario with inline content (admin/testing)
type SeedScenarioInput struct {
	Title         string          `json:"title" binding:"required,max=1000"`
	Description   string          `json:"description" binding:"max=1000"`
	Difficulty    string          `json:"difficulty"`
	EstimatedTime string          `json:"estimated_time"`
	InstanceType  string          `json:"instance_type"`
	Hostname      string          `json:"hostname,omitempty"`
	OsType        string          `json:"os_type"`
	FlagsEnabled  bool            `json:"flags_enabled"`
	GshEnabled    bool            `json:"gsh_enabled"`
	CrashTraps    bool            `json:"crash_traps"`
	IsPublic      bool            `json:"is_public"`
	IntroText     string          `json:"intro_text" binding:"max=65536"`
	FinishText    string          `json:"finish_text" binding:"max=65536"`
	SetupScript   string          `json:"setup_script,omitempty"`
	Steps         []SeedStepInput `json:"steps" binding:"required,min=1"`
}

// SeedStepInput - DTO for a single step in a seed scenario
type SeedStepInput struct {
	Title            string `json:"title" binding:"required,max=1000"`
	TextContent      string `json:"text_content" binding:"max=65536"`
	HintContent      string `json:"hint_content" binding:"max=65536"`
	VerifyScript     string `json:"verify_script"`
	BackgroundScript string `json:"background_script"`
	ForegroundScript string `json:"foreground_script"`
	HasFlag          bool   `json:"has_flag"`
	FlagPath         string `json:"flag_path"`
}

// ScenarioExportStepOutput — full step data including scripts (for export only)
type ScenarioExportStepOutput struct {
	Order            int    `json:"order"`
	Title            string `json:"title"`
	TextContent      string `json:"text_content,omitempty"`
	HintContent      string `json:"hint_content,omitempty"`
	VerifyScript     string `json:"verify_script,omitempty"`
	BackgroundScript string `json:"background_script,omitempty"`
	ForegroundScript string `json:"foreground_script,omitempty"`
	HasFlag          bool   `json:"has_flag"`
	FlagPath         string `json:"flag_path,omitempty"`
	FlagLevel        int    `json:"flag_level,omitempty"`
}

// ScenarioExportOutput — full scenario data for JSON export/re-import
// Designed to match SeedScenarioInput so the exported JSON can be re-imported directly
type ScenarioExportOutput struct {
	Title         string                     `json:"title"`
	Description   string                     `json:"description,omitempty"`
	Difficulty    string                     `json:"difficulty,omitempty"`
	EstimatedTime string                     `json:"estimated_time,omitempty"`
	InstanceType  string                     `json:"instance_type"`
	OsType        string                     `json:"os_type,omitempty"`
	FlagsEnabled  bool                       `json:"flags_enabled"`
	GshEnabled    bool                       `json:"gsh_enabled"`
	CrashTraps    bool                       `json:"crash_traps"`
	IsPublic      bool                       `json:"is_public"`
	IntroText     string                     `json:"intro_text,omitempty"`
	FinishText    string                     `json:"finish_text,omitempty"`
	SetupScript   string                     `json:"setup_script,omitempty"`
	Steps         []ScenarioExportStepOutput `json:"steps"`
}

// ExportScenariosInput — request body for bulk export
type ExportScenariosInput struct {
	IDs []uuid.UUID `json:"ids" binding:"required,min=1"`
}

// LaunchScenarioInput - DTO for direct scenario launch (auto-provisions terminal)
type LaunchScenarioInput struct {
	ScenarioID string `json:"scenario_id" binding:"required"`
	Backend    string `json:"backend,omitempty"`
}

// LaunchScenarioResponse - DTO for launch scenario result
type LaunchScenarioResponse struct {
	TerminalSessionID string `json:"terminal_session_id"`
	ScenarioSessionID string `json:"scenario_session_id"`
	Status            string `json:"status"`
	ProvisioningPhase string `json:"provisioning_phase,omitempty"`
}

// AvailableScenarioOutput - enriched scenario with launchability info
type AvailableScenarioOutput struct {
	ID                      uuid.UUID                    `json:"id"`
	Name                    string                       `json:"name"`
	Title                   string                       `json:"title"`
	Description             string                       `json:"description,omitempty"`
	Difficulty              string                       `json:"difficulty"`
	EstimatedTime           string                       `json:"estimated_time"`
	InstanceType            string                       `json:"instance_type"`
	OsType                  string                       `json:"os_type,omitempty"`
	CompatibleInstanceTypes []ScenarioInstanceTypeOutput `json:"compatible_instance_types,omitempty"`
	Launchable              bool                         `json:"launchable"`
	AvailableInstanceTypes  []string                     `json:"available_instance_types,omitempty"`
	IsPublic                bool                         `json:"is_public"`
	AdminOnly               bool                         `json:"admin_only,omitempty"`
}

// MySessionResponse - DTO for a learner's own scenario session
type MySessionResponse struct {
	ID                uuid.UUID  `json:"id"`
	ScenarioID        uuid.UUID  `json:"scenario_id"`
	ScenarioTitle     string     `json:"scenario_title"`
	Status            string     `json:"status"`
	ProvisioningPhase string     `json:"provisioning_phase,omitempty"`
	Grade             *float64   `json:"grade,omitempty"`
	CurrentStep       int        `json:"current_step"`
	TotalSteps        int        `json:"total_steps"`
	CompletedSteps    int        `json:"completed_steps"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	TerminalSessionID *string    `json:"terminal_session_id,omitempty"`
}
