package scenarioRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func RegisterScenarioStepHint(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioStepHint, dto.CreateScenarioStepHintInput, dto.EditScenarioStepHintInput, dto.ScenarioStepHintOutput](
		service,
		"ScenarioStepHint",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioStepHint, dto.CreateScenarioStepHintInput, dto.EditScenarioStepHintInput, dto.ScenarioStepHintOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioStepHint, dto.CreateScenarioStepHintInput, dto.EditScenarioStepHintInput, dto.ScenarioStepHintOutput]{
				ModelToDto: func(model *models.ScenarioStepHint) (dto.ScenarioStepHintOutput, error) {
					return dto.ScenarioStepHintOutput{
						ID:        model.ID,
						StepID:    model.StepID,
						Level:     model.Level,
						Content:   model.Content,
						CreatedAt: model.CreatedAt,
						UpdatedAt: model.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateScenarioStepHintInput) *models.ScenarioStepHint {
					return &models.ScenarioStepHint{
						StepID:  input.StepID,
						Level:   input.Level,
						Content: input.Content,
					}
				},
				DtoToMap: func(input dto.EditScenarioStepHintInput) map[string]any {
					updates := make(map[string]any)
					if input.Level != nil {
						updates["level"] = *input.Level
					}
					if input.Content != nil {
						updates["content"] = *input.Content
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
				Tag:        "scenario-step-hints",
				EntityName: "ScenarioStepHint",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "List all scenario step hints",
					Description: "Retrieve all progressive hints for scenario steps",
					Tags:        []string{"scenario-step-hints"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get a scenario step hint",
					Description: "Retrieve a specific scenario step hint by ID",
					Tags:        []string{"scenario-step-hints"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create a scenario step hint",
					Description: "Create a new progressive hint for a scenario step",
					Tags:        []string{"scenario-step-hints"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update a scenario step hint",
					Description: "Update an existing scenario step hint",
					Tags:        []string{"scenario-step-hints"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete a scenario step hint",
					Description: "Delete a scenario step hint (admin only)",
					Tags:        []string{"scenario-step-hints"},
					Security:    true,
				},
			},
		},
	)
}
