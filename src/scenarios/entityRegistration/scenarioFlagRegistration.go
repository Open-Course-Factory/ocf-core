package scenarioRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func RegisterScenarioFlag(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioFlag, dto.CreateScenarioFlagInput, dto.EditScenarioFlagInput, dto.ScenarioFlagOutput](
		service,
		"ScenarioFlag",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioFlag, dto.CreateScenarioFlagInput, dto.EditScenarioFlagInput, dto.ScenarioFlagOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioFlag, dto.CreateScenarioFlagInput, dto.EditScenarioFlagInput, dto.ScenarioFlagOutput]{
				ModelToDto: func(model *models.ScenarioFlag) (dto.ScenarioFlagOutput, error) {
					return dto.ScenarioFlagOutput{
						ID:            model.ID,
						SessionID:     model.SessionID,
						StepOrder:     model.StepOrder,
						SubmittedFlag: model.SubmittedFlag,
						SubmittedAt:   model.SubmittedAt,
						IsCorrect:     model.IsCorrect,
						CreatedAt:     model.CreatedAt,
						UpdatedAt:     model.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateScenarioFlagInput) *models.ScenarioFlag {
					return &models.ScenarioFlag{
						SessionID:    input.SessionID,
						StepOrder:    input.StepOrder,
						ExpectedFlag: input.ExpectedFlag,
					}
				},
				DtoToMap: func(input dto.EditScenarioFlagInput) map[string]any {
					updates := make(map[string]any)
					if input.SubmittedFlag != nil {
						updates["submitted_flag"] = *input.SubmittedFlag
					}
					if input.SubmittedAt != nil {
						updates["submitted_at"] = *input.SubmittedAt
					}
					if input.IsCorrect != nil {
						updates["is_correct"] = *input.IsCorrect
					}
					return updates
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "scenario-flags",
				EntityName: "ScenarioFlag",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "List all scenario flags",
					Description: "Retrieve all flag entries for scenario sessions",
					Tags:        []string{"scenario-flags"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get a scenario flag",
					Description: "Retrieve a specific flag entry by ID",
					Tags:        []string{"scenario-flags"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create a scenario flag",
					Description: "Create a new flag entry for a session step",
					Tags:        []string{"scenario-flags"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Submit a flag answer",
					Description: "Submit or update a flag answer for a session step",
					Tags:        []string{"scenario-flags"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete a scenario flag",
					Description: "Delete a flag entry (admin only)",
					Tags:        []string{"scenario-flags"},
					Security:    true,
				},
			},
		},
	)
}
