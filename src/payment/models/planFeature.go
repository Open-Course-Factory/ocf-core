package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// PlanFeature represents a feature that can be assigned to subscription plans.
// Features are organized by category and support different value types.
type PlanFeature struct {
	entityManagementModels.BaseModel
	Key           string `gorm:"uniqueIndex;not null" json:"key" mapstructure:"key"`
	DisplayNameEn string `gorm:"not null" json:"display_name_en" mapstructure:"display_name_en"`
	DisplayNameFr string `gorm:"not null" json:"display_name_fr" mapstructure:"display_name_fr"`
	Description   string `gorm:"type:text" json:"description" mapstructure:"description"`
	Category      string `gorm:"type:varchar(50);not null" json:"category" mapstructure:"category"`
	ValueType     string `gorm:"type:varchar(20);not null;default:'boolean'" json:"value_type" mapstructure:"value_type"`
	Unit          string `gorm:"type:varchar(20)" json:"unit" mapstructure:"unit"`
	DefaultValue  string `gorm:"type:varchar(100);default:'false'" json:"default_value" mapstructure:"default_value"`
	IsActive      bool   `gorm:"default:true" json:"is_active" mapstructure:"is_active"`
}

func (p PlanFeature) GetBaseModel() entityManagementModels.BaseModel {
	return p.BaseModel
}

func (p PlanFeature) GetReferenceObject() string {
	return "PlanFeature"
}
