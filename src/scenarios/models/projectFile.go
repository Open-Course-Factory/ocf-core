package models

import (
	"github.com/google/uuid"
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// ProjectFile represents a reusable file (script, markdown, text, image) stored in the database
type ProjectFile struct {
	entityManagementModels.BaseModel
	Name        string     `gorm:"type:varchar(255);not null" json:"name" mapstructure:"name"`
	RelPath     string     `gorm:"type:varchar(1000)" json:"rel_path,omitempty" mapstructure:"rel_path"`
	ContentType string     `gorm:"type:varchar(50);not null" json:"content_type" mapstructure:"content_type"` // script, markdown, text, image
	MimeType    string     `gorm:"type:varchar(100)" json:"mime_type,omitempty" mapstructure:"mime_type"`     // e.g. image/png, image/jpeg
	Content     string     `gorm:"type:text" json:"-" mapstructure:"content"`                                 // hidden from JSON by default (base64 for images)
	StorageType string     `gorm:"type:varchar(20);not null;default:'database'" json:"storage_type" mapstructure:"storage_type"`
	StorageRef  string     `gorm:"type:varchar(1000)" json:"storage_ref,omitempty" mapstructure:"storage_ref"`
	SizeBytes   int64      `gorm:"default:0" json:"size_bytes" mapstructure:"size_bytes"`
	ScenarioID  *uuid.UUID `gorm:"type:uuid;index" json:"scenario_id,omitempty" mapstructure:"scenario_id"` // links image files to their scenario
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
