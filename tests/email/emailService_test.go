package email_tests

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	emailModels "soli/formations/src/email/models"
	"soli/formations/src/email/services"
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// --- NewEmailService / NewEmailServiceWithDB construction ---

func TestNewEmailService_ReturnsNonNil(t *testing.T) {
	svc := services.NewEmailService()
	assert.NotNil(t, svc)
}

func TestNewEmailServiceWithDB_ReturnsNonNil(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewEmailServiceWithDB(db)
	assert.NotNil(t, svc)
}

// --- SendEmail ---

func TestEmailService_SendEmail_MissingCredentials(t *testing.T) {
	// Ensure SMTP credentials are not set
	os.Unsetenv("SMTP_USERNAME")
	os.Unsetenv("SMTP_PASSWORD")

	svc := services.NewEmailService()
	err := svc.SendEmail("test@example.com", "Subject", "<p>Body</p>")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SMTP credentials not configured")
}

func TestEmailService_SendEmail_InvalidSMTPHost(t *testing.T) {
	// Set credentials but use an invalid SMTP host to test error handling
	os.Setenv("SMTP_HOST", "invalid.host.local")
	os.Setenv("SMTP_PORT", "587")
	os.Setenv("SMTP_USERNAME", "testuser")
	os.Setenv("SMTP_PASSWORD", "testpass")
	defer func() {
		os.Unsetenv("SMTP_HOST")
		os.Unsetenv("SMTP_PORT")
		os.Unsetenv("SMTP_USERNAME")
		os.Unsetenv("SMTP_PASSWORD")
	}()

	svc := services.NewEmailService()
	err := svc.SendEmail("test@example.com", "Subject", "<p>Body</p>")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send email")
}

// --- SendEmailWithAttachment ---

func TestEmailService_SendEmailWithAttachment_MissingCredentials(t *testing.T) {
	os.Unsetenv("SMTP_USERNAME")
	os.Unsetenv("SMTP_PASSWORD")

	svc := services.NewEmailService()
	err := svc.SendEmailWithAttachment("test@example.com", "Subject", "<p>Body</p>", "image.png", "base64data")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SMTP credentials not configured")
}

// --- SendPasswordResetEmail ---

func TestEmailService_SendPasswordResetEmail_WithoutDB_FallbackTemplate(t *testing.T) {
	// Without DB, the method falls back to the hardcoded template
	// It will still fail at SendEmail due to missing SMTP credentials
	os.Unsetenv("SMTP_USERNAME")
	os.Unsetenv("SMTP_PASSWORD")

	svc := services.NewEmailService()
	err := svc.SendPasswordResetEmail("test@example.com", "token123", "https://example.com/reset")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SMTP credentials not configured")
}

func TestEmailService_SendPasswordResetEmail_WithDB_UsesTemplate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	os.Unsetenv("SMTP_USERNAME")
	os.Unsetenv("SMTP_PASSWORD")

	db := freshTestDB(t)
	services.InitDefaultTemplates(db)

	svc := services.NewEmailServiceWithDB(db)
	err := svc.SendPasswordResetEmail("test@example.com", "token123", "https://example.com/reset")
	// It should fail at the SMTP send step, not at template loading
	assert.Error(t, err)
	// The error should propagate from SendEmail (SMTP credentials)
	assert.Contains(t, err.Error(), "SMTP credentials not configured")
}

// --- SendTemplatedEmail ---

func TestEmailService_SendTemplatedEmail_NoTemplateService(t *testing.T) {
	// NewEmailService() creates service without DB, so templateService is nil
	svc := services.NewEmailService()
	err := svc.SendTemplatedEmail("test@example.com", "password_reset", map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "template service not initialized")
}

func TestEmailService_SendTemplatedEmail_TemplateNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewEmailServiceWithDB(db)

	err := svc.SendTemplatedEmail("test@example.com", "nonexistent", map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to render template")
}

func TestEmailService_SendTemplatedEmail_ValidTemplate_FailsAtSMTP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	os.Unsetenv("SMTP_USERNAME")
	os.Unsetenv("SMTP_PASSWORD")

	db := freshTestDB(t)

	// Seed a template
	seedTemplate(t, &emailModels.EmailTemplate{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "test_templated",
		DisplayName: "Test",
		Subject:     "Welcome {{.UserName}}",
		HTMLBody:    "<p>Hello {{.UserName}}</p>",
		IsActive:    true,
	})

	svc := services.NewEmailServiceWithDB(db)
	vars := map[string]interface{}{"UserName": "Bob"}
	err := svc.SendTemplatedEmail("test@example.com", "test_templated", vars)
	assert.Error(t, err)
	// Should fail at SMTP, not at template rendering
	assert.Contains(t, err.Error(), "SMTP credentials not configured")
}

// --- Port 465 (TLS) path ---

func TestEmailService_SendEmail_TLSPort_InvalidHost(t *testing.T) {
	os.Setenv("SMTP_HOST", "invalid.host.local")
	os.Setenv("SMTP_PORT", "465")
	os.Setenv("SMTP_USERNAME", "testuser")
	os.Setenv("SMTP_PASSWORD", "testpass")
	defer func() {
		os.Unsetenv("SMTP_HOST")
		os.Unsetenv("SMTP_PORT")
		os.Unsetenv("SMTP_USERNAME")
		os.Unsetenv("SMTP_PASSWORD")
	}()

	svc := services.NewEmailService()
	err := svc.SendEmail("test@example.com", "Subject", "<p>Body</p>")
	assert.Error(t, err)
	// Should get a TLS connection error
	assert.Contains(t, err.Error(), "failed to connect to SMTP server with TLS")
}
