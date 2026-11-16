// tests/email/emailTemplateGenericRules_test.go
//
// Issue #606: EmailTemplate is written through the generic CRUD routes, so the
// rules must run there — an unparsable subject/body breaks every email sent
// from that template, and deleting a system template breaks the code that
// sends it by name. These drive the REAL generic service with the email
// module's hooks wired the way production wires them.
package email_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	emailDto "soli/formations/src/email/dto"
	registration "soli/formations/src/email/entityRegistration"
	emailHooks "soli/formations/src/email/hooks"
	emailModels "soli/formations/src/email/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	entityServices "soli/formations/src/entityManagement/services"
)

const invalidTemplate = "Hello {{.Name" // unclosed action: html/template refuses to parse it

// setupEmailTemplateEntity registers the real EmailTemplate entity and a fresh
// hook registry holding only the email module's hooks.
func setupEmailTemplateEntity(t *testing.T) entityServices.GenericService {
	t.Helper()
	db := freshTestDB(t)

	casdoor.Enforcer = mocks.NewMockEnforcer()
	if _, ok := ems.GlobalEntityRegistrationService.GetEntityOps("EmailTemplate"); !ok {
		registration.RegisterEmailTemplate(ems.GlobalEntityRegistrationService)
		t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity("EmailTemplate") })
	}

	orig := hooks.GlobalHookRegistry
	hooks.GlobalHookRegistry = hooks.NewHookRegistry()
	t.Cleanup(func() { hooks.GlobalHookRegistry = orig })
	emailHooks.InitEmailHooks(db)

	return entityServices.NewGenericService(db, nil)
}

func seedEmailTemplate(t *testing.T, name string, isSystem bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, sharedTestDB.Create(&emailModels.EmailTemplate{
		BaseModel:   entityManagementModels.BaseModel{ID: id},
		Name:        name,
		DisplayName: name,
		Subject:     "Hello {{.Name}}",
		HTMLBody:    "<p>Hi {{.Name}}</p>",
		IsActive:    true,
		IsSystem:    isSystem,
	}).Error)
	return id
}

func countTemplates(t *testing.T, where string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, sharedTestDB.Model(&emailModels.EmailTemplate{}).Where(where, args...).Count(&n).Error)
	return n
}

// requireClientError asserts the refusal reaches the controller as a 4xx, not
// the generic ENT007/500 a non-entity hook error becomes.
func requireClientError(t *testing.T, err error, wantStatus int) {
	t.Helper()
	require.Error(t, err)
	var entityErr *entityErrors.EntityError
	require.ErrorAs(t, err, &entityErr)
	if wantStatus != 0 {
		assert.Equal(t, wantStatus, entityErr.HTTPStatus, "error: %v", err)
	} else {
		assert.True(t, entityErr.HTTPStatus >= 400 && entityErr.HTTPStatus < 500,
			"want a 4xx, got %d: %v", entityErr.HTTPStatus, err)
	}
}

func validCreateInput(name string) emailDto.CreateTemplateInput {
	return emailDto.CreateTemplateInput{
		Name:        name,
		DisplayName: "Display " + name,
		Subject:     "Hello {{.Name}}",
		HTMLBody:    "<p>Hi {{.Name}}</p>",
		IsActive:    true,
	}
}

func TestEmailTemplateGeneric_CreateInvalidSyntax_Rejected(t *testing.T) {
	cases := map[string]func(*emailDto.CreateTemplateInput){
		"subject": func(in *emailDto.CreateTemplateInput) { in.Subject = invalidTemplate },
		"body":    func(in *emailDto.CreateTemplateInput) { in.HTMLBody = invalidTemplate },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			svc := setupEmailTemplateEntity(t)
			in := validCreateInput("bad_" + field)
			mutate(&in)

			_, err := svc.CreateEntityWithUser(in, "EmailTemplate", "admin-1", "administrator")

			requireClientError(t, err, 400)
			assert.Equal(t, int64(0), countTemplates(t, "name = ?", in.Name), "refused template must not be persisted")
		})
	}
}

func TestEmailTemplateGeneric_CreateValid_Persisted(t *testing.T) {
	svc := setupEmailTemplateEntity(t)

	_, err := svc.CreateEntityWithUser(validCreateInput("good_one"), "EmailTemplate", "admin-1", "administrator")

	require.NoError(t, err)
	assert.Equal(t, int64(1), countTemplates(t, "name = ?", "good_one"))
}

func TestEmailTemplateGeneric_UpdateInvalidSyntax_Rejected(t *testing.T) {
	for _, field := range []string{"subject", "html_body"} {
		t.Run(field, func(t *testing.T) {
			svc := setupEmailTemplateEntity(t)
			id := seedEmailTemplate(t, "to_break", false)

			// The generic PATCH controller hands the service the DtoToMap output.
			err := svc.EditEntityWithUser(id, "EmailTemplate", &emailModels.EmailTemplate{},
				map[string]any{field: invalidTemplate}, "admin-1", "administrator")

			requireClientError(t, err, 400)
			assert.Equal(t, int64(0), countTemplates(t, field+" = ?", invalidTemplate), "refused patch must not be written")
		})
	}
}

func TestEmailTemplateGeneric_UpdateValid_Persisted(t *testing.T) {
	svc := setupEmailTemplateEntity(t)
	id := seedEmailTemplate(t, "to_edit", false)

	err := svc.EditEntityWithUser(id, "EmailTemplate", &emailModels.EmailTemplate{},
		map[string]any{"subject": "Bonjour {{.Name}}"}, "admin-1", "administrator")

	require.NoError(t, err)
	assert.Equal(t, int64(1), countTemplates(t, "id = ? AND subject = ?", id, "Bonjour {{.Name}}"))
}

func TestEmailTemplateGeneric_DeleteSystemTemplate_Rejected(t *testing.T) {
	svc := setupEmailTemplateEntity(t)
	id := seedEmailTemplate(t, "password_reset", true)

	err := svc.DeleteEntityWithUser(id, &emailModels.EmailTemplate{}, true, "admin-1", "administrator")

	requireClientError(t, err, 0)
	assert.Equal(t, int64(1), countTemplates(t, "id = ?", id), "a system template must survive the delete")
}

func TestEmailTemplateGeneric_DeleteNonSystemTemplate_Deleted(t *testing.T) {
	svc := setupEmailTemplateEntity(t)
	id := seedEmailTemplate(t, "custom_one", false)

	err := svc.DeleteEntityWithUser(id, &emailModels.EmailTemplate{}, true, "admin-1", "administrator")

	require.NoError(t, err)
	assert.Equal(t, int64(0), countTemplates(t, "id = ?", id))
}
