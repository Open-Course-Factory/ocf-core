package models

import (
	"log"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ScenarioStep represents a single step within a scenario
type ScenarioStep struct {
	entityManagementModels.BaseModel
	ScenarioID            uuid.UUID `gorm:"type:uuid;not null;index" json:"scenario_id"`
	Order                 int       `gorm:"not null" json:"order"`
	Title                 string    `gorm:"type:varchar(500);not null" json:"title"`
	StepType              string    `gorm:"type:varchar(50);default:'terminal'" json:"step_type"`
	ShowImmediateFeedback bool      `gorm:"default:false" json:"show_immediate_feedback"`
	TextContent           string    `gorm:"type:text" json:"text_content,omitempty"` // markdown
	HintContent           string    `gorm:"type:text" json:"hint_content,omitempty"` // markdown
	VerifyScript          string    `gorm:"type:text" json:"-"`
	BackgroundScript      string    `gorm:"type:text" json:"-"`
	// ForegroundScript is typed into the learner's own shell at the end of the
	// step's provisioning, matching KillerCoda's semantics: the learner watches
	// it run, and anything it changes about the shell — cd, export, a function —
	// persists for them. That is what distinguishes it from BackgroundScript,
	// which runs in a separate non-interactive exec they never see.
	//
	// Delivery is best-effort. It needs a console attached, so a learner who has
	// not opened their terminal simply misses it; the level itself is already
	// provisioned by the time this runs, so nothing is left unsolvable.
	ForegroundScript string `gorm:"type:text" json:"-"`
	// Intro/outro banners. The trainer picks an effect by name and types a
	// line; the engine turns that into an ocf-banner call in the container, so
	// nobody has to write shell to get one. Empty effect or empty text means no
	// banner — both are required for anything to render.
	//
	// These hold an effect NAME and the TEXT, never a rendered asset: tte draws
	// live inside the container, and a step configured with an effect still
	// runs unchanged on an image that has no tte.
	IntroEffect string `gorm:"type:varchar(64)" json:"intro_effect,omitempty" mapstructure:"intro_effect"`
	IntroText   string `gorm:"type:varchar(500)" json:"intro_text,omitempty" mapstructure:"intro_text"`
	OutroEffect string `gorm:"type:varchar(64)" json:"outro_effect,omitempty" mapstructure:"outro_effect"`
	OutroText   string `gorm:"type:varchar(500)" json:"outro_text,omitempty" mapstructure:"outro_text"`
	// BackgroundTimeoutSeconds overrides the engine's timeout for this step's
	// background script. 0 means "use the engine default", which depends on
	// whether the step is the initial setup or a later one.
	BackgroundTimeoutSeconds int `gorm:"default:0" json:"background_timeout_seconds,omitempty" mapstructure:"background_timeout_seconds"`
	// BackgroundAsync forces this step's provisioning off the advance request
	// and into a background goroutine, moving the session to "provisioning"
	// until it finishes. Long timeouts imply it; this flag opts a step in
	// regardless of its timeout.
	BackgroundAsync    bool                   `gorm:"default:false" json:"background_async,omitempty" mapstructure:"background_async"`
	HasFlag            bool                   `gorm:"default:false" json:"has_flag"`
	FlagPath           string                 `gorm:"type:varchar(500)" json:"flag_path,omitempty"` // where to place the flag file in the container
	FlagLevel          int                    `gorm:"default:0" json:"flag_level"`
	VerifyScriptID     *uuid.UUID             `gorm:"type:uuid;index" json:"verify_script_id,omitempty" mapstructure:"verify_script_id"`
	BackgroundScriptID *uuid.UUID             `gorm:"type:uuid;index" json:"background_script_id,omitempty" mapstructure:"background_script_id"`
	ForegroundScriptID *uuid.UUID             `gorm:"type:uuid;index" json:"foreground_script_id,omitempty" mapstructure:"foreground_script_id"`
	TextFileID         *uuid.UUID             `gorm:"type:uuid;index" json:"text_file_id,omitempty" mapstructure:"text_file_id"`
	HintFileID         *uuid.UUID             `gorm:"type:uuid;index" json:"hint_file_id,omitempty" mapstructure:"hint_file_id"`
	Hints              []ScenarioStepHint     `gorm:"foreignKey:StepID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"hints,omitempty"`
	Questions          []ScenarioStepQuestion `gorm:"foreignKey:StepID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"questions,omitempty"`
}

// Implement interfaces for entity management system
func (s ScenarioStep) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioStep) GetReferenceObject() string {
	return "ScenarioStep"
}

// TableName specifies the table name
func (ScenarioStep) TableName() string {
	return "scenario_steps"
}

// Canonical step_type values. The strings are part of the stored data and the
// API contract with the frontend, so they must not be renamed.
const (
	StepTypeTerminal = "terminal"
	StepTypeFlag     = "flag"
)

// NormalizeFlagStep is the one rule linking a step's type to its flag: a step
// carries a flag exactly when it is a flag step.
//
// has_flag still promotes a terminal (or undeclared) step to a flag step,
// because that is how KillerCoda imports and the editor's legacy checkbox say
// "flag step". It never adds a flag to a quiz or info step, and it never
// removes one from a flag step: the editor creates flag steps with has_flag
// false, and those must still get their flag.
func NormalizeFlagStep(stepType string, hasFlag bool) (string, bool) {
	if stepType == "" {
		stepType = StepTypeTerminal
	}
	if hasFlag && stepType == StepTypeTerminal {
		stepType = StepTypeFlag
	}
	return stepType, stepType == StepTypeFlag
}

// BeforeSave applies NormalizeFlagStep to every struct write — create, save,
// seed, import, duplicate. A PATCH writes a column map that this cannot see;
// scenarioHooks.ScenarioStepFlagHook applies the same rule there.
func (s *ScenarioStep) BeforeSave(*gorm.DB) error {
	s.StepType, s.HasFlag = NormalizeFlagStep(s.StepType, s.HasFlag)
	return nil
}

// MigrateFlagStepConsistency applies NormalizeFlagStep to the rows written
// before it existed: terminal steps with a flag were dead ends (Verify refused,
// no flag input), and flag steps without one generated no flag. Idempotent.
func MigrateFlagStepConsistency(db *gorm.DB) {
	repairs := []struct{ what, sql string }{
		{"terminal steps with a flag promoted to flag steps",
			"UPDATE scenario_steps SET step_type = 'flag' WHERE has_flag = true AND (step_type = 'terminal' OR step_type = '' OR step_type IS NULL)"},
		{"untyped steps given the terminal type",
			"UPDATE scenario_steps SET step_type = 'terminal' WHERE step_type = '' OR step_type IS NULL"},
		{"flag steps given their flag",
			"UPDATE scenario_steps SET has_flag = true WHERE step_type = 'flag' AND has_flag = false"},
		{"non-flag steps cleared of a flag",
			"UPDATE scenario_steps SET has_flag = false WHERE step_type <> 'flag' AND has_flag = true"},
	}
	for _, r := range repairs {
		res := db.Exec(r.sql)
		if res.Error != nil {
			log.Printf("[FLAG-STEP-MIGRATION] %s: %v", r.what, res.Error)
			return
		}
		if res.RowsAffected > 0 {
			log.Printf("[FLAG-STEP-MIGRATION] %d %s", res.RowsAffected, r.what)
		}
	}
}
