package dto

import (
	"time"

	"github.com/google/uuid"
)

// SessionResponse - DTO for scenario session information
type SessionResponse struct {
	ID                string  `json:"id"`
	ScenarioID        string  `json:"scenario_id"`
	UserID            string  `json:"user_id"`
	TrainerID         *string `json:"trainer_id,omitempty"`
	TerminalSessionID string  `json:"terminal_session_id"`
	CurrentStep       int     `json:"current_step"`
	Status            string  `json:"status"`
	ProvisioningPhase string  `json:"provisioning_phase,omitempty"`
	// ProvisioningTimeoutSeconds is the running step's effective timeout, set
	// only while Status is "provisioning". A client that polls this endpoint
	// after a reload never saw the advance response, so it needs the ceiling
	// from here to know when to stop waiting.
	ProvisioningTimeoutSeconds int       `json:"provisioning_timeout_seconds,omitempty"`
	Grade                      *float64  `json:"grade,omitempty"`
	StartedAt                  time.Time `json:"started_at"`
	// ScenarioText is the scenario's own prose in the language this session was
	// started in. Served from here rather than left to the client to fetch,
	// because the session is what knows the locale — a client asking the plain
	// scenario endpoint gets the source language and has no way to tell.
	ScenarioText *SessionScenarioText `json:"scenario_text,omitempty"`
}

// SessionScenarioText is the briefing, the farewell, and the names around them.
type SessionScenarioText struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	IntroText   string `json:"intro_text,omitempty"`
	FinishText  string `json:"finish_text,omitempty"`
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
	Locale            string `json:"locale,omitempty"`
}

// StepProvisioningStatus reports what happened to the container after an
// advance. It is embedded in every advance response (and so inlined in their
// JSON) so the three endpoints speak one language about it.
//
// The three fields are mutually exclusive in practice: setup either moved to
// the background, failed inline, or finished inline with nothing to report.
type StepProvisioningStatus struct {
	// NextStepProvisioning is true only when setup actually moved to the
	// background and the session is now "provisioning". The client must poll
	// the session until it clears before showing the step. Setup that ran and
	// finished inline reports nothing — the step is already playable.
	NextStepProvisioning bool `json:"next_step_provisioning,omitempty"`
	// ProvisioningTimeoutSeconds is the step's effective timeout, set only
	// alongside NextStepProvisioning. Clients derive their poll ceiling from
	// it rather than guessing a constant that a long step would outlive.
	ProvisioningTimeoutSeconds int `json:"provisioning_timeout_seconds,omitempty"`
	// NextStepProvisioningFailed is true when setup ran inline and failed. The
	// advance still stands (the flag is burned, it is never rolled back), so
	// this is the client's cue to offer a retry via reprovision-step.
	//
	// Deliberately a bare boolean: background scripts carry flags, passwords
	// and puzzle internals, so no script output may reach the learner. The
	// details stay in the server log.
	NextStepProvisioningFailed bool `json:"next_step_provisioning_failed,omitempty"`
}

// VerifyStepResponse - DTO for verify step results
type VerifyStepResponse struct {
	Passed   bool   `json:"passed"`
	Output   string `json:"output,omitempty"`
	NextStep *int   `json:"next_step,omitempty"`
	StepProvisioningStatus
}

// ReprovisionStepInput - DTO for re-running the current step's setup
type ReprovisionStepInput struct {
	// Force tells the step script to redo work its idempotency markers would
	// otherwise skip (exported as FORCE=1).
	Force bool `json:"force,omitempty"`
}

// ReprovisionStepResponse - DTO for a reprovisioning request
type ReprovisionStepResponse struct {
	StepOrder int `json:"step_order"`
	// Status is the session status right after the call: "provisioning" when
	// the setup runs in the background and the client must poll until it
	// clears, "active" when it already finished.
	Status string `json:"status"`
}

// SubmitFlagInput - DTO for submitting a flag answer
type SubmitFlagInput struct {
	Flag string `json:"flag" binding:"required,max=1000"`
}

// SubmitFlagResponse - DTO for flag submission results
type SubmitFlagResponse struct {
	Correct  bool   `json:"correct"`
	Message  string `json:"message,omitempty"`
	NextStep *int   `json:"next_step,omitempty"`
	StepProvisioningStatus
}

// CurrentStepResponse - DTO for current step information
type CurrentStepResponse struct {
	StepOrder int `json:"step_order"`
	// Position is the step's 1-based index among the scenario's ordered
	// steps — the ONLY value clients may display as "step N of M". Orders
	// are data-driven (0- or 1-based depending on authoring path), so
	// arithmetic on StepOrder renders "Étape 3 / 2" on 1-based scenarios.
	Position int `json:"position"`
	// StepOrders lists every step's Order in display order, so a client can
	// map a display position back to the order it must navigate to.
	StepOrders            []int                 `json:"step_orders,omitempty"`
	TotalSteps            int                   `json:"total_steps"`
	Title                 string                `json:"title"`
	Text                  string                `json:"text,omitempty"`
	Hint                  string                `json:"hint,omitempty"`
	HintsTotalCount       int                   `json:"hints_total_count"`
	HintsRevealed         int                   `json:"hints_revealed"`
	Status                string                `json:"status"`
	HasFlag               bool                  `json:"has_flag"`
	StepType              string                `json:"step_type"`
	TextContent           string                `json:"text_content,omitempty"`
	Questions             []CurrentStepQuestion `json:"questions,omitempty"`
	ShowImmediateFeedback bool                  `json:"show_immediate_feedback"`
}

// CurrentStepQuestion - sanitized public DTO for a quiz question.
// CRITICAL: this DTO MUST NOT include CorrectAnswer or Explanation — those
// would leak the answer to the student. Post-submission results expose them
// via QuizQuestionResult.
type CurrentStepQuestion struct {
	ID           uuid.UUID `json:"id"`
	Order        int       `json:"order"`
	QuestionText string    `json:"question_text"`
	QuestionType string    `json:"question_type"`
	Options      string    `json:"options,omitempty"`
}

// SubmitQuizInput - DTO for submitting a quiz answer set
type SubmitQuizInput struct {
	Answers map[uuid.UUID]string `json:"answers" binding:"required"`
}

// SubmitQuizResponse - DTO for quiz submission results.
// PerQuestionResults is only populated in learning mode
// (show_immediate_feedback=true); exam mode returns the score line alone so
// the correct answers never reach the learner's browser.
type SubmitQuizResponse struct {
	Score              float64              `json:"score"`
	CorrectCount       int                  `json:"correct_count"`
	Total              int                  `json:"total"`
	PerQuestionResults []QuizQuestionResult `json:"per_question_results,omitempty"`
	NextStep           *int                 `json:"next_step,omitempty"`
	StepProvisioningStatus
}

// QuizQuestionResult - per-question result returned after a LEARNING-mode
// submission, where revealing the correct answer and explanation is the
// point. Exam mode omits these entirely (the teacher's flag controls the
// API payload, not just the UI rendering).
type QuizQuestionResult struct {
	QuestionID    uuid.UUID `json:"question_id"`
	Correct       bool      `json:"correct"`
	CorrectAnswer string    `json:"correct_answer,omitempty"`
	Explanation   string    `json:"explanation,omitempty"`
}

// RevealHintResponse - DTO for revealing a progressive hint
type RevealHintResponse struct {
	Level   int    `json:"level"`
	Content string `json:"content"`
	Total   int    `json:"total"`
}

// SeedScenarioInput - DTO for seeding a scenario with inline content (admin/testing)
type SeedScenarioInput struct {
	Title            string `json:"title" binding:"required,max=1000"`
	Description      string `json:"description" binding:"max=1000"`
	Difficulty       string `json:"difficulty"`
	EstimatedTimeMinutes int `json:"estimated_time_minutes"`
	InstanceType     string `json:"instance_type"`
	Hostname         string `json:"hostname,omitempty"`
	OsType           string `json:"os_type"`
	FlagsEnabled     bool   `json:"flags_enabled"`
	AllowedFlagPaths string `json:"allowed_flag_paths,omitempty"`
	CrashTraps       bool   `json:"crash_traps"`
	// SessionUser is the uid the learner's console runs as. Absent means the
	// distribution decides, which is root — fine for every scenario whose
	// lesson is not "the kernel said no".
	SessionUser      *int   `json:"session_user,omitempty"`
	IsPublic         bool   `json:"is_public"`
	IntroText        string `json:"intro_text" binding:"max=65536"`
	FinishText       string `json:"finish_text" binding:"max=65536"`
	SetupScript      string `json:"setup_script,omitempty"`
	// CompatibleInstanceTypes names the distributions this scenario is built
	// for, most preferred first. Leave empty to keep matching on os_type alone.
	CompatibleInstanceTypes []string `json:"compatible_instance_types,omitempty"`
	// RequiredFeatures names the session features the scenario cannot run
	// without — currently "network". Same meaning as the archive importer's
	// field: the ocf-base profile is NIC-less, so a scenario whose setup
	// installs packages must ask for network or apt resolves nothing.
	//
	// The importer learned this and seeding did not, which left the two paths
	// disagreeing about what a scenario is allowed to declare — re-seeding a
	// scenario silently dropped the requirement it was imported with.
	RequiredFeatures []string `json:"required_features,omitempty"`
	// BuildFeatures names features held only while the container is
	// provisioned, then removed. Same meaning as the archive importer's field.
	BuildFeatures []string        `json:"build_features,omitempty"`
	Steps         []SeedStepInput `json:"steps" binding:"required,min=1"`
}

// SeedQuestionInput - DTO for a quiz question inside a SeedStepInput
type SeedQuestionInput struct {
	Order         int    `json:"order"`
	QuestionText  string `json:"question_text"`
	QuestionType  string `json:"question_type"`
	Options       string `json:"options,omitempty"`
	CorrectAnswer string `json:"correct_answer,omitempty"`
	Explanation   string `json:"explanation,omitempty"`
	Points        int    `json:"points,omitempty"`
}

// SeedStepInput - DTO for a single step in a seed scenario
type SeedStepInput struct {
	Title                    string              `json:"title" binding:"required,max=1000"`
	StepType                 string              `json:"step_type,omitempty"`
	ShowImmediateFeedback    bool                `json:"show_immediate_feedback,omitempty"`
	TextContent              string              `json:"text_content" binding:"max=65536"`
	HintContent              string              `json:"hint_content" binding:"max=65536"`
	VerifyScript             string              `json:"verify_script"`
	BackgroundScript         string              `json:"background_script"`
	ForegroundScript         string              `json:"foreground_script"`
	IntroEffect              string              `json:"intro_effect,omitempty"`
	IntroText                string              `json:"intro_text,omitempty" binding:"max=500"`
	OutroEffect              string              `json:"outro_effect,omitempty"`
	OutroText                string              `json:"outro_text,omitempty" binding:"max=500"`
	BackgroundTimeoutSeconds int                 `json:"background_timeout_seconds,omitempty"`
	BackgroundAsync          bool                `json:"background_async,omitempty"`
	HasFlag                  bool                `json:"has_flag"`
	FlagPath                 string              `json:"flag_path"`
	Questions                []SeedQuestionInput `json:"questions,omitempty"`
}

// ScenarioExportStepQuestionOutput — quiz question shape inside a scenario export
type ScenarioExportStepQuestionOutput struct {
	Order         int    `json:"order"`
	QuestionText  string `json:"question_text"`
	QuestionType  string `json:"question_type"`
	Options       string `json:"options,omitempty"`
	CorrectAnswer string `json:"correct_answer,omitempty"`
	Explanation   string `json:"explanation,omitempty"`
	Points        int    `json:"points,omitempty"`
}

// ScenarioExportStepOutput — full step data including scripts (for export only)
type ScenarioExportStepOutput struct {
	Order                    int                                `json:"order"`
	Title                    string                             `json:"title"`
	StepType                 string                             `json:"step_type,omitempty"`
	ShowImmediateFeedback    bool                               `json:"show_immediate_feedback,omitempty"`
	TextContent              string                             `json:"text_content,omitempty"`
	HintContent              string                             `json:"hint_content,omitempty"`
	VerifyScript             string                             `json:"verify_script,omitempty"`
	BackgroundScript         string                             `json:"background_script,omitempty"`
	ForegroundScript         string                             `json:"foreground_script,omitempty"`
	IntroEffect              string                             `json:"intro_effect,omitempty"`
	IntroText                string                             `json:"intro_text,omitempty"`
	OutroEffect              string                             `json:"outro_effect,omitempty"`
	OutroText                string                             `json:"outro_text,omitempty"`
	BackgroundTimeoutSeconds int                                `json:"background_timeout_seconds,omitempty"`
	BackgroundAsync          bool                               `json:"background_async,omitempty"`
	HasFlag                  bool                               `json:"has_flag"`
	FlagPath                 string                             `json:"flag_path,omitempty"`
	FlagLevel                int                                `json:"flag_level,omitempty"`
	Questions                []ScenarioExportStepQuestionOutput `json:"questions,omitempty"`
}

// ScenarioExportOutput — full scenario data for JSON export/re-import
// Designed to match SeedScenarioInput so the exported JSON can be re-imported directly
type ScenarioExportOutput struct {
	Title            string                     `json:"title"`
	Description      string                     `json:"description,omitempty"`
	Difficulty       string                     `json:"difficulty,omitempty"`
	EstimatedTimeMinutes int                    `json:"estimated_time_minutes,omitempty"`
	InstanceType     string                     `json:"instance_type"`
	OsType           string                     `json:"os_type,omitempty"`
	FlagsEnabled     bool                       `json:"flags_enabled"`
	AllowedFlagPaths string                     `json:"allowed_flag_paths,omitempty"`
	CrashTraps       bool                       `json:"crash_traps"`
	SessionUser      *int                       `json:"session_user,omitempty"`
	IsPublic         bool                       `json:"is_public"`
	IntroText        string                     `json:"intro_text,omitempty"`
	FinishText       string                     `json:"finish_text,omitempty"`
	SetupScript      string                     `json:"setup_script,omitempty"`
	Steps            []ScenarioExportStepOutput `json:"steps"`
}

// ExportScenariosInput — request body for bulk export
type ExportScenariosInput struct {
	IDs []uuid.UUID `json:"ids" binding:"required,min=1"`
}

// LaunchScenarioInput - DTO for direct scenario launch (auto-provisions terminal)
type LaunchScenarioInput struct {
	ScenarioID     string `json:"scenario_id" binding:"required"`
	Backend        string `json:"backend,omitempty"`
	OrganizationID string `json:"organization_id,omitempty"`

	// Locale the session is built and read in. Empty means the scenario's own
	// content, which is what every client sends today.
	//
	// Deliberately not defaulted from the account's preferred language here.
	// Choosing a default needs to know which locales this scenario actually
	// offers — a scenario with no French would otherwise be stamped French and
	// silently serve English anyway — and that belongs with the launcher, which
	// is where a learner picks.
	Locale string `json:"locale,omitempty"`
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
	EstimatedTimeMinutes    int                          `json:"estimated_time_minutes"`
	InstanceType            string                       `json:"instance_type"`
	OsType                  string                       `json:"os_type,omitempty"`
	CompatibleInstanceTypes []ScenarioInstanceTypeOutput `json:"compatible_instance_types,omitempty"`
	RequiredFeatures        []string                     `json:"required_features,omitempty"`
	Launchable              bool                         `json:"launchable"`
	BlockReason             string                       `json:"block_reason,omitempty"`
	AvailableInstanceTypes  []string                     `json:"available_instance_types,omitempty"`
	// ActiveSessionID and ActiveTerminalSessionID name the run the learner
	// already has of this scenario, when there is one. They come from the same
	// evaluation the launch path uses, so a card can offer Resume from this one
	// response instead of correlating it against a separately fetched session
	// list that may be older than the page.
	ActiveSessionID         string `json:"active_session_id,omitempty"`
	ActiveTerminalSessionID string `json:"active_terminal_session_id,omitempty"`
	// ResolvedDistribution and ResolvedSize are what a launch would actually
	// use, after image matching and size fallback. InstanceType above is only
	// what the scenario asked for, and the two diverge routinely — an unknown
	// size falls back, and a scenario naming no image gets whichever the
	// backend offers. Callers must display these, never re-derive them:
	// duplicating the resolution rules in a client is how a launcher came to
	// label a distribution name as an unknown machine size.
	// Both are empty when resolution failed; BlockReason says why.
	ResolvedDistribution string `json:"resolved_distribution,omitempty"`
	ResolvedSize         string `json:"resolved_size,omitempty"`
	IsPublic             bool   `json:"is_public"`
	AdminOnly            bool   `json:"admin_only,omitempty"`

	// AvailableLocales are the languages this scenario can actually be started
	// in — the ones whose translation is complete and current, never merely
	// declared. Empty means single-language, and a launcher should show no
	// picker rather than one with a single entry.
	//
	// Like ResolvedDistribution above, this is resolved here on purpose:
	// deciding in a client which languages look ready enough is how the same
	// rule ends up written twice and drifting.
	AvailableLocales []string `json:"available_locales,omitempty"`

	// LocalisedText is this card's title and description in each language it is
	// offered in. Sent with the card rather than fetched per choice: the picker
	// sits beside these words, and a card that blanked while it asked the
	// server would make choosing a language feel like loading a page.
	LocalisedText map[string]ScenarioText `json:"localised_text,omitempty"`
}

// ScenarioText is what a catalogue card shows, in one language.
type ScenarioText struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// AssignmentProgressItem - per-scenario progress summary for a group's
// Scenarios tab. One item per scenario that has qualifying (non-preview)
// sessions from active group members. AvgGrade is nil when no member has
// completed the scenario.
type AssignmentProgressItem struct {
	ScenarioID     uuid.UUID `json:"scenario_id"`
	TotalCount     int       `json:"total_count"`
	CompletedCount int       `json:"completed_count"`
	AvgGrade       *float64  `json:"avg_grade"`
}

// MySessionResponse - DTO for a learner's own scenario session
type MySessionResponse struct {
	ID                uuid.UUID  `json:"id"`
	ScenarioID        uuid.UUID  `json:"scenario_id"`
	ScenarioTitle     string     `json:"scenario_title"`
	TrainerID         *string    `json:"trainer_id,omitempty"`
	Status            string     `json:"status"`
	ProvisioningPhase string     `json:"provisioning_phase,omitempty"`
	Grade             *float64   `json:"grade,omitempty"`
	CurrentStep       int        `json:"current_step"`
	TotalSteps        int        `json:"total_steps"`
	CompletedSteps    int        `json:"completed_steps"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	TerminalSessionID *string    `json:"terminal_session_id,omitempty"`
	// Resumable answers the only question the launcher actually asks of this
	// list: can the learner still return to this run? Status alone cannot say
	// so — a session stays "active" in the database until something notices its
	// terminal is gone — and a client re-deriving it from Status offered a
	// Resume button into a container deleted the day before, with no way to
	// start a fresh run. Computed with the same rule the launch path enforces.
	Resumable         bool       `json:"resumable"`
}
