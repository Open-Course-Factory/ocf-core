package models

import (
	"encoding/json"
	"fmt"
	"slices"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// Scenario represents a hands-on interactive lab scenario
type Scenario struct {
	entityManagementModels.BaseModel
	Name           string     `gorm:"type:varchar(255);not null;index" json:"name"`
	Title          string     `gorm:"type:varchar(500);not null" json:"title"`
	Description    string     `gorm:"type:text" json:"description,omitempty"`
	Difficulty     string     `gorm:"type:varchar(50)" json:"difficulty"`        // beginner, intermediate, advanced
	// Minutes, not prose. It used to hold "90 minutes", which is a sentence in
	// one language: a French learner read it in English, and the three spellings
	// in use ("90 minutes", "10m", "") could not be compared or summed.
	EstimatedTimeMinutes int  `gorm:"not null;default:0" json:"estimated_time_minutes"`
	InstanceType   string     `gorm:"type:varchar(255);not null" json:"instance_type"` // Incus image id
	Hostname       string     `gorm:"type:varchar(63)" json:"hostname,omitempty"`
	OsType           string     `gorm:"type:varchar(50)" json:"os_type,omitempty"`  // deb, rpm, apk, pacman
	RequiredFeatures string     `gorm:"type:text" json:"required_features,omitempty"`
	SourceType       string     `gorm:"type:varchar(50)" json:"source_type"`       // git, upload, builtin
	GitRepository  string     `gorm:"type:varchar(1000)" json:"git_repository,omitempty"`
	GitBranch      string     `gorm:"type:varchar(255);default:'main'" json:"git_branch"`
	SourcePath     string     `gorm:"type:varchar(1000)" json:"source_path,omitempty"`
	FlagsEnabled   bool       `gorm:"default:false" json:"flags_enabled"`
	FlagSecret       string     `gorm:"type:varchar(500)" json:"-"` // never exposed in API
	AllowedFlagPaths string     `gorm:"type:text" json:"allowed_flag_paths,omitempty" mapstructure:"allowed_flag_paths"` // comma-separated allowed path prefixes; empty = defaults
	CrashTraps     bool       `gorm:"default:false" json:"crash_traps"`
	// PortExposureAllowed lets learners of this scenario publish a port of
	// the lab at a public URL, when their plan allows it too. Off by default:
	// a lab is not a hosting account.
	PortExposureAllowed bool  `gorm:"default:false" json:"port_exposure_allowed"`
	// Machine says where a run of this scenario runs: one of the
	// ScenarioMachine* values. The platform switch, the learner's plan and the
	// run's assignment decide the rest at launch.
	Machine string `gorm:"type:varchar(20);default:'incus'" json:"machine" mapstructure:"machine"`
	// MachineApps is the JSON array of the apps a run's machine in the
	// learner's browser offers to install, from LocalMachineApps, in the
	// launcher of unkernel's desktop (see MachineDisplay).
	MachineApps string `gorm:"type:text" json:"machine_apps,omitempty" mapstructure:"machine_apps"`
	// MachineDisplay says what a run's machine in the learner's browser shows:
	// one of the ScenarioMachineDisplay* values. MachineApps matter only on a
	// desktop; a terminal scenario keeps them and ignores them.
	MachineDisplay string `gorm:"type:varchar(20);default:'terminal'" json:"machine_display,omitempty" mapstructure:"machine_display"`
	// MachineBoot is the JSON object of the LocalMachineBoot keys this
	// scenario sets over the platform's boot defaults; empty sets none.
	MachineBoot string `gorm:"type:text" json:"machine_boot,omitempty" mapstructure:"machine_boot"`
	Objectives    string     `gorm:"type:text" json:"objectives,omitempty"`
	Prerequisites  string     `gorm:"type:text" json:"prerequisites,omitempty"`
	IntroText      string     `gorm:"type:text" json:"intro_text,omitempty"`
	FinishText     string     `gorm:"type:text" json:"finish_text,omitempty"`
	CreatedByID    string     `gorm:"type:varchar(255)" json:"created_by_id"`
	OrganizationID *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty"`
	IsPublic       bool       `gorm:"default:false" json:"is_public"`
	// BuildFeatures names session features attached only while the container is
	// provisioned, then removed. A scenario that installs packages needs the
	// network to build and never again; asking for it in RequiredFeatures buys a
	// whole session of egress and puts the scenario out of reach of every plan
	// that does not grant internet access.
	BuildFeatures  string     `gorm:"type:text" json:"build_features,omitempty" mapstructure:"build_features"`

	// SessionUser is the uid the learner's console runs as, when the scenario
	// needs one of its own. Nil means the distribution decides, which is root
	// for every image we ship.
	//
	// A scenario asks for this when its subject is the kernel refusing access:
	// root holds CAP_DAC_OVERRIDE, so it reads a mode-000 file and walks into a
	// mode-000 directory, and a mission about `chmod` is then a mission about
	// nothing. Pointer rather than int because 0 is a uid — a scenario that
	// says "root" and one that says nothing are different scenarios.
	SessionUser *int `gorm:"" json:"session_user,omitempty" mapstructure:"session_user"`

	// DefaultLocale names the language the scenario's own fields are written
	// in. Empty means the content is simply itself — which is every scenario
	// that exists today, and stays valid.
	DefaultLocale string `gorm:"type:varchar(10)" json:"default_locale,omitempty" mapstructure:"default_locale"`

	// Locales is the JSON array of languages this scenario is offered in,
	// including the default. It is a declaration, not a measurement: a language
	// listed here may still be half translated, which is what the coverage
	// report is for.
	Locales string `gorm:"type:text" json:"locales,omitempty" mapstructure:"locales"`
	SetupScript    string     `gorm:"type:text" json:"setup_script,omitempty"`
	// Deprecated: content lives inline only (InlineFileBackedContent empties
	// these at startup). Kept for rollback safety; drop in the next release.
	SetupScriptID  *uuid.UUID `gorm:"type:uuid;index" json:"setup_script_id,omitempty" mapstructure:"setup_script_id"`
	IntroFileID    *uuid.UUID `gorm:"type:uuid;index" json:"intro_file_id,omitempty" mapstructure:"intro_file_id"`
	FinishFileID   *uuid.UUID `gorm:"type:uuid;index" json:"finish_file_id,omitempty" mapstructure:"finish_file_id"`
	// Archiving retires the scenario without deleting it — see NotArchived.
	entityManagementModels.Archivable

	// Relations
	Steps                  []ScenarioStep         `gorm:"foreignKey:ScenarioID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"steps,omitempty"`
	CompatibleInstanceTypes []ScenarioInstanceType `gorm:"foreignKey:ScenarioID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"compatible_instance_types,omitempty"`
}

// Implement interfaces for entity management system
func (s Scenario) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s Scenario) GetReferenceObject() string {
	return "Scenario"
}

// GetRequiredFeatures parses the RequiredFeatures JSON array field
func (s Scenario) GetRequiredFeatures() ([]string, error) {
	if s.RequiredFeatures == "" {
		return nil, nil
	}
	var features []string
	if err := json.Unmarshal([]byte(s.RequiredFeatures), &features); err != nil {
		return nil, fmt.Errorf("invalid required_features format (must be JSON array): %w", err)
	}
	return features, nil
}

// GetFeaturesMap returns required features as a map[string]bool for composed sessions
func (s Scenario) GetFeaturesMap() (map[string]bool, error) {
	return featureNamesToMap(s.GetRequiredFeatures())
}

// GetLocales parses the Locales JSON array. A scenario that declares none is
// single-language, and callers must treat that as "nothing to translate"
// rather than defaulting to a language nobody asked for.
func (s Scenario) GetLocales() ([]string, error) {
	if s.Locales == "" {
		return nil, nil
	}
	var locales []string
	if err := json.Unmarshal([]byte(s.Locales), &locales); err != nil {
		return nil, fmt.Errorf("invalid locales format (must be JSON array): %w", err)
	}
	return locales, nil
}

// HasFlagSteps reports whether any of the loaded steps is a flag step, by the
// NormalizeFlagStep rule. Steps must be loaded: a scenario read without them
// reports false.
func (s Scenario) HasFlagSteps() bool {
	return slices.ContainsFunc(s.Steps, func(st ScenarioStep) bool { return st.StepType == StepTypeFlag })
}

// GetBuildFeatures parses the BuildFeatures JSON array field
func (s Scenario) GetBuildFeatures() ([]string, error) {
	if s.BuildFeatures == "" {
		return nil, nil
	}
	var features []string
	if err := json.Unmarshal([]byte(s.BuildFeatures), &features); err != nil {
		return nil, fmt.Errorf("invalid build_features format (must be JSON array): %w", err)
	}
	return features, nil
}

// GetBuildFeaturesMap returns build-only features as a map[string]bool for
// composed sessions. These are attached to provision the container and taken
// away once it is built — see the build_features field.
func (s Scenario) GetBuildFeaturesMap() (map[string]bool, error) {
	return featureNamesToMap(s.GetBuildFeatures())
}

// featureNamesToMap converts a feature-name list into the map shape the
// composed-session API takes, propagating the parse error unchanged.
//
// Taking the accessor's two return values directly keeps required and build
// features on one conversion: they are the same list in the same shape sent to
// the same endpoint, and two copies would be free to drift.
func featureNamesToMap(features []string, err error) (map[string]bool, error) {
	if err != nil {
		return nil, err
	}
	if len(features) == 0 {
		return nil, nil
	}
	m := make(map[string]bool, len(features))
	for _, f := range features {
		m[f] = true
	}
	return m, nil
}

// TableName specifies the table name
func (Scenario) TableName() string {
	return "scenarios"
}

// Where a scenario runs. Incus: a container, always. Local: a machine in the
// learner's browser tab, for plans that allow it. ByAccount: a machine in the
// browser for plans that make it their default, a container for the others.
const (
	ScenarioMachineIncus     = "incus"
	ScenarioMachineLocal     = "local"
	ScenarioMachineByAccount = "by_account"
)

// ScenarioMachineProblem says what is wrong with a scenario's machine value,
// "" when nothing is. Empty is the column default, Incus.
func ScenarioMachineProblem(machine string) string {
	switch machine {
	case "", ScenarioMachineIncus, ScenarioMachineLocal, ScenarioMachineByAccount:
		return ""
	}
	return fmt.Sprintf("machine %q is not one of %s, %s, %s", machine, ScenarioMachineIncus, ScenarioMachineLocal, ScenarioMachineByAccount)
}

// MachineMayRunLocally says a scenario whose machine value is machine may run
// in the learner's browser, for some plan; an Incus scenario never does.
func MachineMayRunLocally(machine string) bool {
	return machine == ScenarioMachineLocal || machine == ScenarioMachineByAccount
}

// MachineFieldProblem is what is wrong with one of a scenario's machine fields.
type MachineFieldProblem struct {
	Field, Problem string
}

// MachineProblems checks the scenario's machine fields: machine,
// machine_apps, machine_display and machine_boot. None when all are valid.
func (s Scenario) MachineProblems() []MachineFieldProblem {
	var problems []MachineFieldProblem
	for _, check := range []MachineFieldProblem{
		{"machine", ScenarioMachineProblem(s.Machine)},
		{"machine_apps", ScenarioMachineAppsProblem(s.MachineApps)},
		{"machine_display", ScenarioMachineDisplayProblem(s.MachineDisplay)},
		{"machine_boot", ScenarioMachineBootProblem(s.MachineBoot)},
	} {
		if check.Problem != "" {
			problems = append(problems, check)
		}
	}
	return problems
}

// What a run's machine in the learner's browser shows: a terminal, or
// unkernel's desktop with the scenario's MachineApps in its launcher.
const (
	ScenarioMachineDisplayTerminal = "terminal"
	ScenarioMachineDisplayDesktop  = "desktop"
)

// NeedsDesktop says a run of the scenario shows unkernel's desktop, which
// the platform's desktop switch gates (LocalMachineDesktopEnabled).
func (s *Scenario) NeedsDesktop() bool {
	return s.MachineDisplay == ScenarioMachineDisplayDesktop
}

// ScenarioMachineDisplayProblem says what is wrong with a scenario's
// machine_display value, "" when nothing is. Empty is the column default, a
// terminal.
func ScenarioMachineDisplayProblem(display string) string {
	switch display {
	case "", ScenarioMachineDisplayTerminal, ScenarioMachineDisplayDesktop:
		return ""
	}
	return fmt.Sprintf("machine_display %q is not one of %s, %s", display, ScenarioMachineDisplayTerminal, ScenarioMachineDisplayDesktop)
}

// LocalMachineApps is every app a scenario may offer on a machine in the
// learner's browser: ids of the launcher of unkernel's desktop page. Only
// tools a lab may need, and only those that install from Debian alone, since
// the machine reaches nothing but our Debian mirror. Never games, browsers,
// media or agents.
var LocalMachineApps = []string{
	"nvim", "tmux", "go", "rust", "python",
	"nginx", "postgres", "sqlite", "valkey",
	"htop", "mc", "thunar", "l3afpad", "galculator",
}

// GetMachineApps parses the MachineApps JSON array. None means none.
func (s Scenario) GetMachineApps() ([]string, error) {
	if s.MachineApps == "" {
		return nil, nil
	}
	var apps []string
	if err := json.Unmarshal([]byte(s.MachineApps), &apps); err != nil {
		return nil, fmt.Errorf("invalid machine_apps format (must be JSON array): %w", err)
	}
	return apps, nil
}

// ScenarioMachineAppsProblem says what is wrong with a scenario's
// machine_apps value, "" when nothing is.
func ScenarioMachineAppsProblem(machineApps string) string {
	apps, err := Scenario{MachineApps: machineApps}.GetMachineApps()
	if err != nil {
		return err.Error()
	}
	for _, app := range apps {
		if !slices.Contains(LocalMachineApps, app) {
			return fmt.Sprintf("machine_apps: %q is not an app a scenario may offer", app)
		}
	}
	return ""
}
