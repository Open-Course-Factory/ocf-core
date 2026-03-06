package models

import (
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// ScenarioAssignment represents an assignment of a scenario to a group or organization
type ScenarioAssignment struct {
	entityManagementModels.BaseModel
	ScenarioID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"scenario_id" mapstructure:"scenario_id"`
	GroupID        *uuid.UUID `gorm:"type:uuid;index" json:"group_id,omitempty" mapstructure:"group_id"`
	OrganizationID *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty" mapstructure:"organization_id"`
	Scope          string     `gorm:"type:varchar(50);not null" json:"scope" mapstructure:"scope"` // "group" or "org"
	CreatedByID    string     `gorm:"type:varchar(255);not null" json:"created_by_id" mapstructure:"created_by_id"`
	Deadline       *time.Time `json:"deadline,omitempty" mapstructure:"deadline"`
	IsActive       bool       `gorm:"default:true" json:"is_active" mapstructure:"is_active"`

	// Relations
	Scenario Scenario `gorm:"foreignKey:ScenarioID" json:"scenario,omitempty"`
}

func (s ScenarioAssignment) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioAssignment) GetReferenceObject() string {
	return "ScenarioAssignment"
}

func (ScenarioAssignment) TableName() string {
	return "scenario_assignments"
}
