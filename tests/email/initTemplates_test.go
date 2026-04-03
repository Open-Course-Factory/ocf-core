package email_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	emailModels "soli/formations/src/email/models"
	"soli/formations/src/email/services"
)

func TestInitDefaultTemplates_CreatesExpectedTemplates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	services.InitDefaultTemplates(db)

	var templates []emailModels.EmailTemplate
	err := db.Find(&templates).Error
	require.NoError(t, err)

	assert.Len(t, templates, 3)

	// Collect names
	names := make(map[string]bool)
	for _, tmpl := range templates {
		names[tmpl.Name] = true
	}

	assert.True(t, names["password_reset"], "password_reset template should exist")
	assert.True(t, names["welcome"], "welcome template should exist")
	assert.True(t, names["email_verification"], "email_verification template should exist")
}

func TestInitDefaultTemplates_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	// Call twice
	services.InitDefaultTemplates(db)
	services.InitDefaultTemplates(db)

	var count int64
	err := db.Model(&emailModels.EmailTemplate{}).Count(&count).Error
	require.NoError(t, err)

	// Should still have exactly 3 templates, not 6
	assert.Equal(t, int64(3), count)
}

func TestInitDefaultTemplates_RequiredFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	services.InitDefaultTemplates(db)

	var templates []emailModels.EmailTemplate
	err := db.Find(&templates).Error
	require.NoError(t, err)

	for _, tmpl := range templates {
		assert.NotEmpty(t, tmpl.Name, "Name should not be empty for template %s", tmpl.DisplayName)
		assert.NotEmpty(t, tmpl.DisplayName, "DisplayName should not be empty for template %s", tmpl.Name)
		assert.NotEmpty(t, tmpl.Description, "Description should not be empty for template %s", tmpl.Name)
		assert.NotEmpty(t, tmpl.Subject, "Subject should not be empty for template %s", tmpl.Name)
		assert.NotEmpty(t, tmpl.HTMLBody, "HTMLBody should not be empty for template %s", tmpl.Name)
		assert.NotEmpty(t, tmpl.Variables, "Variables should not be empty for template %s", tmpl.Name)
		assert.True(t, tmpl.IsActive, "All default templates should be active: %s", tmpl.Name)
	}
}

func TestInitDefaultTemplates_SystemTemplateFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	services.InitDefaultTemplates(db)

	// password_reset and email_verification are system templates, welcome is not
	var passwordReset emailModels.EmailTemplate
	err := db.Where("name = ?", "password_reset").First(&passwordReset).Error
	require.NoError(t, err)
	assert.True(t, passwordReset.IsSystem, "password_reset should be a system template")

	var emailVerification emailModels.EmailTemplate
	err = db.Where("name = ?", "email_verification").First(&emailVerification).Error
	require.NoError(t, err)
	assert.True(t, emailVerification.IsSystem, "email_verification should be a system template")

	var welcome emailModels.EmailTemplate
	err = db.Where("name = ?", "welcome").First(&welcome).Error
	require.NoError(t, err)
	assert.False(t, welcome.IsSystem, "welcome should not be a system template")
}
