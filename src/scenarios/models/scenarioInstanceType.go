package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// ScenarioInstanceType represents a compatible instance type for a scenario (multi-OS support)
type ScenarioInstanceType struct {
	entityManagementModels.BaseModel
	ScenarioID   uuid.UUID `gorm:"type:uuid;not null;index" json:"scenario_id" mapstructure:"scenario_id"`
	InstanceType string    `gorm:"type:varchar(255);not null" json:"instance_type" mapstructure:"instance_type"`
	OsType       string    `gorm:"type:varchar(50)" json:"os_type,omitempty" mapstructure:"os_type"`
	Priority     int       `gorm:"default:0" json:"priority" mapstructure:"priority"`
}

// Implement interfaces for entity management system
func (s ScenarioInstanceType) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s ScenarioInstanceType) GetReferenceObject() string {
	return "ScenarioInstanceType"
}

// TableName specifies the table name
func (ScenarioInstanceType) TableName() string {
	return "scenario_instance_types"
}
