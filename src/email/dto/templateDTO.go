package dto

import (
	"time"

	"github.com/google/uuid"
)


// CreateTemplateInput represents the input for creating an email template
type CreateTemplateInput struct {
	Name        string `json:"name" binding:"required"`
	DisplayName string `json:"display_name" binding:"required"`
	Description string `json:"description"`
	Subject     string `json:"subject" binding:"required"`
	HTMLBody    string `json:"html_body" binding:"required"`
	Variables   string `json:"variables"`
	IsActive    bool   `json:"is_active"`
	IsSystem    bool   `json:"is_system"`
}

// UpdateTemplateInput represents the input for updating an email template
type UpdateTemplateInput struct {
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Subject     string `json:"subject"`
	HTMLBody    string `json:"html_body"`
	Variables   string `json:"variables"`
	IsActive    *bool  `json:"is_active"` // Pointer to allow null
}

// EmailTemplateOutput represents the output DTO for email templates
type EmailTemplateOutput struct {
	ID           uuid.UUID  `json:"id"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Name         string     `json:"name"`
	DisplayName  string     `json:"display_name"`
	Description  string     `json:"description"`
	Subject      string     `json:"subject"`
	HTMLBody     string     `json:"html_body"`
	Variables    string     `json:"variables"`
	IsActive     bool       `json:"is_active"`
	IsSystem     bool       `json:"is_system"`
	LastTestedAt *time.Time `json:"last_tested_at,omitempty"`
}

// TestEmailInput represents the input for sending a test email
type TestEmailInput struct {
	Email string `json:"email" binding:"required,email"`
}

// TemplateResponse represents the response for template operations (DEPRECATED - kept for backward compat)
type TemplateResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// TemplateListResponse represents the response for listing templates (DEPRECATED - kept for backward compat)
type TemplateListResponse struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message"`
	Templates interface{} `json:"templates"`
}
