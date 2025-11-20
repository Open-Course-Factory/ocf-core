package models

import (
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
)

// EmailTemplate represents an email template in the system
type EmailTemplate struct {
	entityManagementModels.BaseModel

	// Template identification
	Name        string `gorm:"type:varchar(255);not null;uniqueIndex" json:"name"` // e.g., "password_reset"
	DisplayName string `gorm:"type:varchar(255);not null" json:"display_name"`     // e.g., "Password Reset"
	Description string `gorm:"type:text" json:"description"`

	// Template content
	Subject  string `gorm:"type:varchar(500);not null" json:"subject"`
	HTMLBody string `gorm:"type:text;not null" json:"html_body"`
	Variables   string `gorm:"type:text" json:"variables"` // JSON array of available variables
	IsActive    bool   `gorm:"default:true" json:"is_active"`
	IsSystem    bool   `gorm:"default:false" json:"is_system"` // System templates cannot be deleted
	LastTestedAt *time.Time `json:"last_tested_at,omitempty"`
}

// TableName specifies the table name for EmailTemplate
func (EmailTemplate) TableName() string {
	return "email_templates"
}

// EmailTemplateVariable represents a variable that can be used in templates
type EmailTemplateVariable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Example     string `json:"example"`
}
