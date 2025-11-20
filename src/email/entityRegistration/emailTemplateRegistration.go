package registration

import (
	"soli/formations/src/email/dto"
	"soli/formations/src/email/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type EmailTemplateRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s EmailTemplateRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
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
	}
}

func (s EmailTemplateRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		template := ptr.(*models.EmailTemplate)
		return dto.EmailTemplateOutput{
			ID:           template.ID,
			CreatedAt:    template.CreatedAt,
			UpdatedAt:    template.UpdatedAt,
			Name:         template.Name,
			DisplayName:  template.DisplayName,
			Description:  template.Description,
			Subject:      template.Subject,
			HTMLBody:     template.HTMLBody,
			Variables:    template.Variables,
			IsActive:     template.IsActive,
			IsSystem:     template.IsSystem,
			LastTestedAt: template.LastTestedAt,
		}, nil
	})
}

func (s EmailTemplateRegistration) EntityInputDtoToEntityModel(input any) any {
	templateInput, ok := input.(dto.CreateTemplateInput)
	if !ok {
		ptrTemplateInput := input.(*dto.CreateTemplateInput)
		templateInput = *ptrTemplateInput
	}

	return &models.EmailTemplate{
		Name:        templateInput.Name,
		DisplayName: templateInput.DisplayName,
		Description: templateInput.Description,
		Subject:     templateInput.Subject,
		HTMLBody:    templateInput.HTMLBody,
		Variables:   templateInput.Variables,
		IsActive:    templateInput.IsActive,
		IsSystem:    templateInput.IsSystem,
	}
}

func (s EmailTemplateRegistration) EntityDtoToMap(input any) map[string]any {
	updateInput, ok := input.(dto.UpdateTemplateInput)
	if !ok {
		ptrUpdateInput := input.(*dto.UpdateTemplateInput)
		updateInput = *ptrUpdateInput
	}

	updateMap := make(map[string]any)

	if updateInput.DisplayName != "" {
		updateMap["display_name"] = updateInput.DisplayName
	}
	if updateInput.Description != "" {
		updateMap["description"] = updateInput.Description
	}
	if updateInput.Subject != "" {
		updateMap["subject"] = updateInput.Subject
	}
	if updateInput.HTMLBody != "" {
		updateMap["html_body"] = updateInput.HTMLBody
	}
	if updateInput.Variables != "" {
		updateMap["variables"] = updateInput.Variables
	}
	if updateInput.IsActive != nil {
		updateMap["is_active"] = *updateInput.IsActive
	}

	return updateMap
}

func (s EmailTemplateRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.EmailTemplate{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateTemplateInput{},
			OutputDto:      dto.EmailTemplateOutput{},
			InputEditDto:   dto.UpdateTemplateInput{},
		},
	}
}
