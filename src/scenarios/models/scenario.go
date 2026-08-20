package models

import (
	"encoding/json"
	"fmt"
	"time"

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
	EstimatedTime  string     `gorm:"type:varchar(100)" json:"estimated_time"`   // e.g. "30m", "1h"
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
	// Deprecated: nothing reads GshEnabled. The in-terminal gsh helper was
	// dropped — ScenarioPanel already carries goal, hints, verify, flag and
	// quiz. The column, the DTOs and the import/export plumbing stay so that
	// archives written before the drop still round-trip byte-for-byte; the
	// editor surface was removed from ocf-front. Do not build on this flag.
	GshEnabled     bool       `gorm:"default:false" json:"gsh_enabled"`
	CrashTraps     bool       `gorm:"default:false" json:"crash_traps"`
	Objectives     string     `gorm:"type:text" json:"objectives,omitempty"`
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
	SetupScript    string     `gorm:"type:text" json:"setup_script,omitempty"`
	SetupScriptID  *uuid.UUID `gorm:"type:uuid;index" json:"setup_script_id,omitempty" mapstructure:"setup_script_id"`
	IntroFileID    *uuid.UUID `gorm:"type:uuid;index" json:"intro_file_id,omitempty" mapstructure:"intro_file_id"`
	FinishFileID   *uuid.UUID `gorm:"type:uuid;index" json:"finish_file_id,omitempty" mapstructure:"finish_file_id"`
	// ArchivedAt retires the scenario without deleting it — see NotArchived.
	ArchivedAt     *time.Time `gorm:"index" json:"archived_at,omitempty" mapstructure:"archived_at"`

	// Relations
	Steps                  []ScenarioStep         `gorm:"foreignKey:ScenarioID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"steps,omitempty"`
	CompatibleInstanceTypes []ScenarioInstanceType `gorm:"foreignKey:ScenarioID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"compatible_instance_types,omitempty"`
}

// Implement interfaces for entity management system
func (s Scenario) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

// IsArchived reports whether the scenario has been retired.
func (s Scenario) IsArchived() bool {
	return s.ArchivedAt != nil
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
