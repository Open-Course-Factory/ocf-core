package email_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	emailModels "soli/formations/src/email/models"
	"soli/formations/src/email/services"
	entityManagementModels "soli/formations/src/entityManagement/models"
)

func createTemplateService(t *testing.T) services.TemplateService {
	t.Helper()
	db := freshTestDB(t)
	return services.NewTemplateService(db)
}

// seedTemplate inserts a template directly into the DB for test setup.
func seedTemplate(t *testing.T, tmpl *emailModels.EmailTemplate) {
	t.Helper()
	if tmpl.ID == uuid.Nil {
		tmpl.ID = uuid.New()
	}
	err := sharedTestDB.Create(tmpl).Error
	require.NoError(t, err)
}

// --- GetTemplate ---

func TestTemplateService_GetTemplate_Found(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := createTemplateService(t)

	seedTemplate(t, &emailModels.EmailTemplate{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "welcome",
		DisplayName: "Welcome",
		Subject:     "Hello",
		HTMLBody:    "<p>Hi</p>",
		IsActive:    true,
	})

	tmpl, err := svc.GetTemplate("welcome")
	assert.NoError(t, err)
	assert.NotNil(t, tmpl)
	assert.Equal(t, "welcome", tmpl.Name)
}

func TestTemplateService_GetTemplate_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := createTemplateService(t)

	tmpl, err := svc.GetTemplate("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, tmpl)
	assert.Contains(t, err.Error(), "template not found")
}

func TestTemplateService_GetTemplate_InactiveNotReturned(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := createTemplateService(t)

	id := uuid.New()
	seedTemplate(t, &emailModels.EmailTemplate{
		BaseModel:   entityManagementModels.BaseModel{ID: id},
		Name:        "inactive_tmpl",
		DisplayName: "Inactive",
		Subject:     "Subject",
		HTMLBody:    "<p>body</p>",
		IsActive:    true,
	})
	// Deactivate via raw update to bypass GORM zero-value skipping
	sharedTestDB.Model(&emailModels.EmailTemplate{}).Where("id = ?", id).Update("is_active", false)

	tmpl, err := svc.GetTemplate("inactive_tmpl")
	assert.Error(t, err)
	assert.Nil(t, tmpl)
}

// --- RenderTemplate ---

func TestTemplateService_RenderTemplate_VariableSubstitution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := createTemplateService(t)

	seedTemplate(t, &emailModels.EmailTemplate{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "render_test",
		DisplayName: "Render Test",
		Subject:     "Hello {{.UserName}}",
		HTMLBody:    "<p>Welcome {{.UserName}} to {{.Platform}}</p>",
		IsActive:    true,
	})

	vars := map[string]any{
		"UserName": "Alice",
		"Platform": "OCF",
	}

	subject, body, err := svc.RenderTemplate("render_test", vars)
	assert.NoError(t, err)
	assert.Equal(t, "Hello Alice", subject)
	assert.Equal(t, "<p>Welcome Alice to OCF</p>", body)
}

func TestTemplateService_RenderTemplate_MissingVars(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := createTemplateService(t)

	seedTemplate(t, &emailModels.EmailTemplate{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "missing_vars",
		DisplayName: "Missing Vars",
		Subject:     "Hello {{.UserName}}",
		HTMLBody:    "<p>Welcome</p>",
		IsActive:    true,
	})

	// html/template with map[string]interface{} renders missing keys as empty string
	subject, body, err := svc.RenderTemplate("missing_vars", map[string]any{})
	assert.NoError(t, err)
	assert.Equal(t, "Hello ", subject)
	assert.Equal(t, "<p>Welcome</p>", body)
}

func TestTemplateService_RenderTemplate_TemplateNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := createTemplateService(t)

	_, _, err := svc.RenderTemplate("nonexistent", map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "template not found")
}

func TestTemplateService_RenderTemplate_BadBodySyntax(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	// Seed a template with bad HTML body syntax directly in DB
	// (bypassing the EmailTemplate hook which validates)
	db := freshTestDB(t)
	svc := services.NewTemplateService(db)

	tmpl := &emailModels.EmailTemplate{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "bad_body",
		DisplayName: "Bad Body",
		Subject:     "OK Subject",
		HTMLBody:    "{{.Unclosed",
		IsActive:    true,
	}
	err := db.Create(tmpl).Error
	require.NoError(t, err)

	_, _, err = svc.RenderTemplate("bad_body", map[string]any{})
	assert.Error(t, err)
}

// --- TestTemplate ---

func TestTemplateService_TestTemplate_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := createTemplateService(t)

	err := svc.TestTemplate(uuid.New(), "test@example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "template not found")
}

func TestTemplateService_TestTemplate_SMTPNotConfigured(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := createTemplateService(t)

	id := uuid.New()
	seedTemplate(t, &emailModels.EmailTemplate{
		BaseModel:   entityManagementModels.BaseModel{ID: id},
		Name:        "test_smtp",
		DisplayName: "Test SMTP",
		Subject:     "Test Subject",
		HTMLBody:    "<p>Test body</p>",
		Variables:   `[{"name":"Var1","description":"desc","example":"val1"}]`,
		IsActive:    true,
	})

	// Without SMTP_USERNAME and SMTP_PASSWORD env vars set,
	// the email service should fail with credentials error
	err := svc.TestTemplate(id, "test@example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send test email")
}

func TestTemplateService_TestTemplate_InvalidVariablesJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := createTemplateService(t)

	id := uuid.New()
	// Insert directly to bypass validation
	tmpl := &emailModels.EmailTemplate{
		BaseModel:   entityManagementModels.BaseModel{ID: id},
		Name:        "bad_vars_json",
		DisplayName: "Bad Vars",
		Subject:     "Subject",
		HTMLBody:    "<p>Body</p>",
		Variables:   `{invalid json`,
		IsActive:    true,
	}
	err := sharedTestDB.Create(tmpl).Error
	require.NoError(t, err)

	err = svc.TestTemplate(id, "test@example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse template variables")
}
