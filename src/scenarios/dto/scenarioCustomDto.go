package dto

// ImportScenarioInput - DTO for importing a scenario from a git repository
type ImportScenarioInput struct {
	GitRepository string `json:"git_repository" binding:"required"`
	GitBranch     string `json:"git_branch,omitempty"` // defaults to "main"
	SourcePath    string `json:"source_path,omitempty"`
}

// StartScenarioInput - DTO for starting a scenario session
type StartScenarioInput struct {
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
	Correct bool   `json:"correct"`
	Message string `json:"message,omitempty"`
}

// CurrentStepResponse - DTO for current step information
type CurrentStepResponse struct {
	StepOrder int    `json:"step_order"`
	Title     string `json:"title"`
	Text      string `json:"text,omitempty"`
	Hint      string `json:"hint,omitempty"`
	Status    string `json:"status"`
	HasFlag   bool   `json:"has_flag"`
}
