package models

import (
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
	OsType         string     `gorm:"type:varchar(50)" json:"os_type,omitempty"`  // deb, rpm, apk, pacman
	SourceType     string     `gorm:"type:varchar(50)" json:"source_type"`       // git, upload, builtin
	GitRepository  string     `gorm:"type:varchar(1000)" json:"git_repository,omitempty"`
	GitBranch      string     `gorm:"type:varchar(255);default:'main'" json:"git_branch"`
	SourcePath     string     `gorm:"type:varchar(1000)" json:"source_path,omitempty"`
	FlagsEnabled   bool       `gorm:"default:false" json:"flags_enabled"`
	FlagSecret     string     `gorm:"type:varchar(500)" json:"-"` // never exposed in API
	GshEnabled     bool       `gorm:"default:false" json:"gsh_enabled"`
	CrashTraps     bool       `gorm:"default:false" json:"crash_traps"`
	IntroText      string     `gorm:"type:text" json:"intro_text,omitempty"`
	FinishText     string     `gorm:"type:text" json:"finish_text,omitempty"`
	CreatedByID    string     `gorm:"type:varchar(255)" json:"created_by_id"`
	OrganizationID *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty"`
	SetupScriptID  *uuid.UUID `gorm:"type:uuid;index" json:"setup_script_id,omitempty" mapstructure:"setup_script_id"`
	IntroFileID    *uuid.UUID `gorm:"type:uuid;index" json:"intro_file_id,omitempty" mapstructure:"intro_file_id"`
	FinishFileID   *uuid.UUID `gorm:"type:uuid;index" json:"finish_file_id,omitempty" mapstructure:"finish_file_id"`

	// Relations
	Steps []ScenarioStep `gorm:"foreignKey:ScenarioID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"steps,omitempty"`
}

// Implement interfaces for entity management system
func (s Scenario) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s Scenario) GetReferenceObject() string {
	return "Scenario"
}

// TableName specifies the table name
func (Scenario) TableName() string {
	return "scenarios"
}
