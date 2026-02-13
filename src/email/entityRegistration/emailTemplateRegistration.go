package registration

import (
	"soli/formations/src/email/dto"
	"soli/formations/src/email/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

func RegisterEmailTemplate(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.EmailTemplate, dto.CreateTemplateInput, dto.UpdateTemplateInput, dto.EmailTemplateOutput](
		service,
		"EmailTemplate",
		entityManagementInterfaces.TypedEntityRegistration[models.EmailTemplate, dto.CreateTemplateInput, dto.UpdateTemplateInput, dto.EmailTemplateOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.EmailTemplate, dto.CreateTemplateInput, dto.UpdateTemplateInput, dto.EmailTemplateOutput]{
				ModelToDto: func(model *models.EmailTemplate) (dto.EmailTemplateOutput, error) {
					return dto.EmailTemplateOutput{
						ID:           model.ID,
						CreatedAt:    model.CreatedAt,
						UpdatedAt:    model.UpdatedAt,
						Name:         model.Name,
						DisplayName:  model.DisplayName,
						Description:  model.Description,
						Subject:      model.Subject,
						HTMLBody:     model.HTMLBody,
						Variables:    model.Variables,
						IsActive:     model.IsActive,
						IsSystem:     model.IsSystem,
						LastTestedAt: model.LastTestedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateTemplateInput) *models.EmailTemplate {
					return &models.EmailTemplate{
						Name:        input.Name,
						DisplayName: input.DisplayName,
						Description: input.Description,
						Subject:     input.Subject,
						HTMLBody:    input.HTMLBody,
						Variables:   input.Variables,
						IsActive:    input.IsActive,
						IsSystem:    input.IsSystem,
					}
				},
				DtoToMap: func(input dto.UpdateTemplateInput) map[string]any {
					updateMap := make(map[string]any)
					if input.DisplayName != "" {
						updateMap["display_name"] = input.DisplayName
					}
					if input.Description != "" {
						updateMap["description"] = input.Description
					}
					if input.Subject != "" {
						updateMap["subject"] = input.Subject
					}
					if input.HTMLBody != "" {
						updateMap["html_body"] = input.HTMLBody
					}
					if input.Variables != "" {
						updateMap["variables"] = input.Variables
					}
					if input.IsActive != nil {
						updateMap["is_active"] = *input.IsActive
					}
					return updateMap
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "email-templates",
				EntityName: "EmailTemplate",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get all email templates",
					Description: "Returns the list of all email templates",
					Tags:        []string{"email-templates"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get an email template",
					Description: "Returns the details of a specific email template",
					Tags:        []string{"email-templates"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create an email template",
					Description: "Creates a new email template",
					Tags:        []string{"email-templates"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update an email template",
					Description: "Modifies an existing email template",
					Tags:        []string{"email-templates"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete an email template",
					Description: "Deletes an email template (cannot delete system templates)",
					Tags:        []string{"email-templates"},
					Security:    true,
				},
			},
		},
	)
}
