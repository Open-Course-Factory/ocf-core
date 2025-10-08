package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// Feature represents a global feature toggle
type Feature struct {
	entityManagementModels.BaseModel
	Key         string `gorm:"type:varchar(100);uniqueIndex;not null" json:"key"`
	Name        string `gorm:"type:varchar(200);not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	Enabled     bool   `gorm:"default:true" json:"enabled"`
	Category    string `gorm:"type:varchar(50)" json:"category"` // e.g., "modules", "features"
	Module      string `gorm:"type:varchar(100)" json:"module"`  // e.g., "courses", "labs", "terminals"
}

// FeatureDefinition defines a feature that a module wants to register
type FeatureDefinition struct {
	Key         string
	Name        string
	Description string
	Enabled     bool   // Default enabled state
	Category    string // e.g., "modules", "features"
	Module      string // Module name that owns this feature
}
