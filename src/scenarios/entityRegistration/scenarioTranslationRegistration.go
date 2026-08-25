package scenarioRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

// Translations are Member-writable so a trainer can translate their own
// scenarios. The step-translation stamp hook runs on every write, which is what
// keeps the source hash out of the caller's hands.
func RegisterScenarioStepTranslation(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioStepTranslation, dto.CreateScenarioStepTranslationInput, dto.EditScenarioStepTranslationInput, dto.ScenarioStepTranslationOutput](
		service,
		"ScenarioStepTranslation",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioStepTranslation, dto.CreateScenarioStepTranslationInput, dto.EditScenarioStepTranslationInput, dto.ScenarioStepTranslationOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioStepTranslation, dto.CreateScenarioStepTranslationInput, dto.EditScenarioStepTranslationInput, dto.ScenarioStepTranslationOutput]{
				ModelToDto: func(model *models.ScenarioStepTranslation) (dto.ScenarioStepTranslationOutput, error) {
					return dto.ScenarioStepTranslationOutput{
						ID:          model.ID,
						StepID:      model.StepID,
						Locale:      model.Locale,
						Title:       model.Title,
						TextContent: model.TextContent,
						HintContent: model.HintContent,
						IntroText:   model.IntroText,
						OutroText:   model.OutroText,
						SourceHash:  model.SourceHash,
						CreatedAt:   model.CreatedAt,
						UpdatedAt:   model.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateScenarioStepTranslationInput) *models.ScenarioStepTranslation {
					// SourceHash is absent on purpose: the stamp hook sets it.
					return &models.ScenarioStepTranslation{
						StepID:      input.StepID,
						Locale:      input.Locale,
						Title:       input.Title,
						TextContent: input.TextContent,
						HintContent: input.HintContent,
						IntroText:   input.IntroText,
						OutroText:   input.OutroText,
					}
				},
				DtoToMap: func(input dto.EditScenarioStepTranslationInput) map[string]any {
					updates := make(map[string]any)
					// Written even when empty: clearing a translated field back
					// to nothing is how a translator says "not done after all",
					// and it must fall back to the default rather than stick.
					if input.Title != nil {
						updates["title"] = *input.Title
					}
					if input.TextContent != nil {
						updates["text_content"] = *input.TextContent
					}
					if input.HintContent != nil {
						updates["hint_content"] = *input.HintContent
					}
					if input.IntroText != nil {
						updates["intro_text"] = *input.IntroText
					}
					if input.OutroText != nil {
						updates["outro_text"] = *input.OutroText
					}
					return updates
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "scenario-step-translations",
				EntityName: "ScenarioStepTranslation",
				GetAll:     &entityManagementInterfaces.SwaggerOperation{Summary: "List step translations", Description: "Retrieve translated step content", Tags: []string{"scenario-step-translations"}, Security: true},
				GetOne:     &entityManagementInterfaces.SwaggerOperation{Summary: "Get a step translation", Description: "Retrieve one translated step", Tags: []string{"scenario-step-translations"}, Security: true},
				Create:     &entityManagementInterfaces.SwaggerOperation{Summary: "Translate a step", Description: "Write a step's content in one locale", Tags: []string{"scenario-step-translations"}, Security: true},
				Update:     &entityManagementInterfaces.SwaggerOperation{Summary: "Update a step translation", Description: "Revise translated step content", Tags: []string{"scenario-step-translations"}, Security: true},
				Delete:     &entityManagementInterfaces.SwaggerOperation{Summary: "Delete a step translation", Description: "Remove a locale's version of a step", Tags: []string{"scenario-step-translations"}, Security: true},
			},
		},
	)
}

func RegisterScenarioTranslation(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioTranslation, dto.CreateScenarioTranslationInput, dto.EditScenarioTranslationInput, dto.ScenarioTranslationOutput](
		service,
		"ScenarioTranslation",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioTranslation, dto.CreateScenarioTranslationInput, dto.EditScenarioTranslationInput, dto.ScenarioTranslationOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioTranslation, dto.CreateScenarioTranslationInput, dto.EditScenarioTranslationInput, dto.ScenarioTranslationOutput]{
				ModelToDto: func(model *models.ScenarioTranslation) (dto.ScenarioTranslationOutput, error) {
					return dto.ScenarioTranslationOutput{
						ID:            model.ID,
						ScenarioID:    model.ScenarioID,
						Locale:        model.Locale,
						Title:         model.Title,
						Description:   model.Description,
						Objectives:    model.Objectives,
						Prerequisites: model.Prerequisites,
						IntroText:     model.IntroText,
						FinishText:    model.FinishText,
						CreatedAt:     model.CreatedAt,
						UpdatedAt:     model.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateScenarioTranslationInput) *models.ScenarioTranslation {
					return &models.ScenarioTranslation{
						ScenarioID:    input.ScenarioID,
						Locale:        input.Locale,
						Title:         input.Title,
						Description:   input.Description,
						Objectives:    input.Objectives,
						Prerequisites: input.Prerequisites,
						IntroText:     input.IntroText,
						FinishText:    input.FinishText,
					}
				},
				DtoToMap: func(input dto.EditScenarioTranslationInput) map[string]any {
					updates := make(map[string]any)
					if input.Title != nil {
						updates["title"] = *input.Title
					}
					if input.Description != nil {
						updates["description"] = *input.Description
					}
					if input.Objectives != nil {
						updates["objectives"] = *input.Objectives
					}
					if input.Prerequisites != nil {
						updates["prerequisites"] = *input.Prerequisites
					}
					if input.IntroText != nil {
						updates["intro_text"] = *input.IntroText
					}
					if input.FinishText != nil {
						updates["finish_text"] = *input.FinishText
					}
					return updates
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "scenario-translations",
				EntityName: "ScenarioTranslation",
				GetAll:     &entityManagementInterfaces.SwaggerOperation{Summary: "List scenario translations", Description: "Retrieve translated scenario fields", Tags: []string{"scenario-translations"}, Security: true},
				GetOne:     &entityManagementInterfaces.SwaggerOperation{Summary: "Get a scenario translation", Description: "Retrieve one translated scenario", Tags: []string{"scenario-translations"}, Security: true},
				Create:     &entityManagementInterfaces.SwaggerOperation{Summary: "Translate a scenario", Description: "Write a scenario's fields in one locale", Tags: []string{"scenario-translations"}, Security: true},
				Update:     &entityManagementInterfaces.SwaggerOperation{Summary: "Update a scenario translation", Description: "Revise translated scenario fields", Tags: []string{"scenario-translations"}, Security: true},
				Delete:     &entityManagementInterfaces.SwaggerOperation{Summary: "Delete a scenario translation", Description: "Remove a locale's version of a scenario", Tags: []string{"scenario-translations"}, Security: true},
			},
		},
	)
}
