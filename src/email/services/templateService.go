package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"time"

	"github.com/google/uuid"
	"soli/formations/src/email/models"
	"soli/formations/src/utils"

	"gorm.io/gorm"
)

type TemplateService interface {
	GetTemplate(name string) (*models.EmailTemplate, error)
	RenderTemplate(name string, variables map[string]any) (subject string, body string, err error)
	TestTemplate(id uuid.UUID, testEmail string) error
}

type templateService struct {
	db           *gorm.DB
	emailService EmailService
}

func NewTemplateService(db *gorm.DB) TemplateService {
	return &templateService{
		db:           db,
		emailService: NewEmailService(),
	}
}

// GetTemplate retrieves a template by name
func (s *templateService) GetTemplate(name string) (*models.EmailTemplate, error) {
	var tmpl models.EmailTemplate
	if err := s.db.Where("name = ? AND is_active = ?", name, true).First(&tmpl).Error; err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}
	return &tmpl, nil
}

// RenderTemplate renders a template with the provided variables
func (s *templateService) RenderTemplate(name string, variables map[string]any) (subject string, body string, err error) {
	tmpl, err := s.GetTemplate(name)
	if err != nil {
		return "", "", err
	}

	return s.renderTemplateContent(tmpl.Subject, tmpl.HTMLBody, variables)
}

// ValidateTemplateSyntax parses one template field the way the renderer does;
// parse-only, since execution errors depend on the variables of each send.
func ValidateTemplateSyntax(tmpl string) error {
	_, err := template.New("").Parse(tmpl)
	return err
}

// renderTemplateContent renders subject and body with variables
func (s *templateService) renderTemplateContent(subjectTmpl, bodyTmpl string, variables map[string]any) (subject string, body string, err error) {
	// Render subject
	subjectTemplate, err := template.New("subject").Parse(subjectTmpl)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse subject template: %w", err)
	}

	var subjectBuf bytes.Buffer
	if err := subjectTemplate.Execute(&subjectBuf, variables); err != nil {
		return "", "", fmt.Errorf("failed to render subject: %w", err)
	}
	subject = subjectBuf.String()

	// Render body
	bodyTemplate, err := template.New("body").Parse(bodyTmpl)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse body template: %w", err)
	}

	var bodyBuf bytes.Buffer
	if err := bodyTemplate.Execute(&bodyBuf, variables); err != nil {
		return "", "", fmt.Errorf("failed to render body: %w", err)
	}
	body = bodyBuf.String()

	return subject, body, nil
}

// TestTemplate sends a test email using the specified template
func (s *templateService) TestTemplate(id uuid.UUID, testEmail string) error {
	var tmpl models.EmailTemplate
	if err := s.db.Where("id = ?", id).First(&tmpl).Error; err != nil {
		return fmt.Errorf("template not found: %w", err)
	}

	// Parse variables to get example values
	var variables []models.EmailTemplateVariable
	if tmpl.Variables != "" {
		if err := json.Unmarshal([]byte(tmpl.Variables), &variables); err != nil {
			return fmt.Errorf("failed to parse template variables: %w", err)
		}
	}

	// Build example data from variables
	exampleData := make(map[string]any)
	for _, v := range variables {
		exampleData[v.Name] = v.Example
	}

	// Render template
	subject, body, err := s.renderTemplateContent(tmpl.Subject, tmpl.HTMLBody, exampleData)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	// Send test email
	if err := s.emailService.SendEmail(testEmail, subject, body); err != nil {
		return fmt.Errorf("failed to send test email: %w", err)
	}

	// Update last tested timestamp
	now := time.Now()
	tmpl.LastTestedAt = &now
	s.db.Save(&tmpl)

	utils.Info("Test email sent to: %s", testEmail)
	return nil
}
