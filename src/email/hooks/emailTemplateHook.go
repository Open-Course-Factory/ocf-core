package emailHooks

import (
	"log"

	"soli/formations/src/email/models"
	"soli/formations/src/email/services"
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"

	"gorm.io/gorm"
)

// EmailTemplateHook enforces the template rules on the generic CRUD routes
// (issue #606): an unparsable subject or body breaks every email sent from the
// template, and a system template is sent by name from code, so it must stay.
type EmailTemplateHook struct {
	hooks.BaseHook
}

func NewEmailTemplateHook() hooks.Hook {
	return &EmailTemplateHook{
		BaseHook: hooks.BaseHook{
			Name:       "email_template_rules",
			EntityName: "EmailTemplate",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate, hooks.BeforeDelete},
			Enabled:    true,
			Priority:   5,
		},
	}
}

func (h *EmailTemplateHook) Execute(ctx *hooks.HookContext) error {
	if ctx.HookType == hooks.BeforeDelete {
		if tmpl, ok := ctx.NewEntity.(*models.EmailTemplate); ok && tmpl.IsSystem {
			return entityErrors.NewConflictError("cannot delete a system email template")
		}
		return nil
	}

	fields := map[string]string{}
	switch v := ctx.NewEntity.(type) {
	case *models.EmailTemplate:
		fields["subject"], fields["html_body"] = v.Subject, v.HTMLBody
	case map[string]any:
		// A patch is checked on the fields it sends only, so a stored legacy
		// row stays editable on its other fields.
		for _, key := range []string{"subject", "html_body"} {
			if s, ok := v[key].(string); ok {
				fields[key] = s
			}
		}
	}

	for field, tmpl := range fields {
		if err := services.ValidateTemplateSyntax(tmpl); err != nil {
			return entityErrors.NewValidationError(field, err.Error())
		}
	}
	return nil
}

func (h *EmailTemplateHook) ShouldExecute(ctx *hooks.HookContext) bool {
	return h.Enabled
}

// InitEmailHooks registers the email module's entity hooks.
func InitEmailHooks(db *gorm.DB) {
	if err := hooks.GlobalHookRegistry.RegisterHook(NewEmailTemplateHook()); err != nil {
		log.Printf("❌ Failed to register email template hook: %v", err)
	}
}
