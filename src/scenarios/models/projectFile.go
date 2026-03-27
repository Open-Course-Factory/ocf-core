package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// ProjectFile represents a reusable file (script, markdown, text) stored in the database
type ProjectFile struct {
	entityManagementModels.BaseModel
	Filename    string `gorm:"type:varchar(255);not null" json:"filename" mapstructure:"filename"`
	RelPath     string `gorm:"type:varchar(1000)" json:"rel_path,omitempty" mapstructure:"rel_path"`
	ContentType string `gorm:"type:varchar(50);not null" json:"content_type" mapstructure:"content_type"` // script, markdown, text
	Content     string `gorm:"type:text" json:"-" mapstructure:"content"`                                 // hidden from JSON by default
	StorageType string `gorm:"type:varchar(20);not null;default:'database'" json:"storage_type" mapstructure:"storage_type"`
	StorageRef  string `gorm:"type:varchar(1000)" json:"storage_ref,omitempty" mapstructure:"storage_ref"`
	SizeBytes   int64  `gorm:"default:0" json:"size_bytes" mapstructure:"size_bytes"`
}

// Implement interfaces for entity management system
func (p ProjectFile) GetBaseModel() entityManagementModels.BaseModel {
	return p.BaseModel
}

func (p ProjectFile) GetReferenceObject() string {
	return "ProjectFile"
}

// TableName specifies the table name
func (ProjectFile) TableName() string {
	return "project_files"
}
