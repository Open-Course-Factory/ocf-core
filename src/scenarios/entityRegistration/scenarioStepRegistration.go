package scenarioRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func RegisterScenarioStep(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioStep, dto.CreateScenarioStepInput, dto.EditScenarioStepInput, dto.ScenarioStepOutput](
		service,
		"ScenarioStep",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioStep, dto.CreateScenarioStepInput, dto.EditScenarioStepInput, dto.ScenarioStepOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioStep, dto.CreateScenarioStepInput, dto.EditScenarioStepInput, dto.ScenarioStepOutput]{
				ModelToDto: func(model *models.ScenarioStep) (dto.ScenarioStepOutput, error) {
					return dto.ScenarioStepOutput{
						ID:               model.ID,
						ScenarioID:       model.ScenarioID,
						Order:            model.Order,
						Title:            model.Title,
						TextContent:      model.TextContent,
						HintContent:      model.HintContent,
						VerifyScript:     model.VerifyScript,
						BackgroundScript: model.BackgroundScript,
						ForegroundScript: model.ForegroundScript,
						HasFlag:          model.HasFlag,
						FlagPath:         model.FlagPath,
						FlagLevel:        model.FlagLevel,
						CreatedAt:        model.CreatedAt,
						UpdatedAt:        model.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateScenarioStepInput) *models.ScenarioStep {
					return &models.ScenarioStep{
						ScenarioID:       input.ScenarioID,
						Order:            input.Order,
						Title:            input.Title,
						TextContent:      input.TextContent,
						HintContent:      input.HintContent,
						VerifyScript:     input.VerifyScript,
						BackgroundScript: input.BackgroundScript,
						ForegroundScript: input.ForegroundScript,
						HasFlag:          input.HasFlag,
						FlagPath:         input.FlagPath,
						FlagLevel:        input.FlagLevel,
					}
				},
				DtoToMap: func(input dto.EditScenarioStepInput) map[string]any {
					updates := make(map[string]any)
					if input.Order != nil {
						updates["order"] = *input.Order
					}
					if input.Title != nil {
						updates["title"] = *input.Title
					}
					if input.TextContent != nil {
						updates["text_content"] = *input.TextContent
					}
					if input.HintContent != nil {
						updates["hint_content"] = *input.HintContent
					}
					if input.VerifyScript != nil {
						updates["verify_script"] = *input.VerifyScript
					}
					if input.BackgroundScript != nil {
						updates["background_script"] = *input.BackgroundScript
					}
					if input.ForegroundScript != nil {
						updates["foreground_script"] = *input.ForegroundScript
					}
					if input.HasFlag != nil {
						updates["has_flag"] = *input.HasFlag
					}
					if input.FlagPath != nil {
						updates["flag_path"] = *input.FlagPath
					}
					if input.FlagLevel != nil {
						updates["flag_level"] = *input.FlagLevel
					}
					return updates
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Admin): "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "scenario-steps",
				EntityName: "ScenarioStep",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "List all scenario steps",
					Description: "Retrieve all steps for scenarios",
					Tags:        []string{"scenario-steps"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get a scenario step",
					Description: "Retrieve a specific scenario step by ID",
					Tags:        []string{"scenario-steps"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create a scenario step",
					Description: "Create a new step within a scenario",
					Tags:        []string{"scenario-steps"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update a scenario step",
					Description: "Update an existing scenario step",
					Tags:        []string{"scenario-steps"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete a scenario step",
					Description: "Delete a scenario step",
					Tags:        []string{"scenario-steps"},
					Security:    true,
				},
			},
		},
	)
}
