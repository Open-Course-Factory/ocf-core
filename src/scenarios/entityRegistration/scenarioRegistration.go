package scenarioRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func RegisterScenario(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Scenario, dto.CreateScenarioInput, dto.EditScenarioInput, dto.ScenarioOutput](
		service,
		"Scenario",
		entityManagementInterfaces.TypedEntityRegistration[models.Scenario, dto.CreateScenarioInput, dto.EditScenarioInput, dto.ScenarioOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Scenario, dto.CreateScenarioInput, dto.EditScenarioInput, dto.ScenarioOutput]{
				ModelToDto: func(model *models.Scenario) (dto.ScenarioOutput, error) {
					output := dto.ScenarioOutput{
						ID:             model.ID,
						Name:           model.Name,
						Title:          model.Title,
						Description:    model.Description,
						Difficulty:     model.Difficulty,
						EstimatedTime:  model.EstimatedTime,
						InstanceType:   model.InstanceType,
						SourceType:     model.SourceType,
						GitRepository:  model.GitRepository,
						GitBranch:      model.GitBranch,
						SourcePath:     model.SourcePath,
						FlagsEnabled:   model.FlagsEnabled,
						GshEnabled:     model.GshEnabled,
						CrashTraps:     model.CrashTraps,
						IntroText:      model.IntroText,
						FinishText:     model.FinishText,
						CreatedByID:    model.CreatedByID,
						OrganizationID: model.OrganizationID,
						CreatedAt:      model.CreatedAt,
						UpdatedAt:      model.UpdatedAt,
					}

					if len(model.Steps) > 0 {
						steps := make([]dto.ScenarioStepOutput, 0, len(model.Steps))
						for _, step := range model.Steps {
							steps = append(steps, dto.ScenarioStepOutput{
								ID:               step.ID,
								ScenarioID:       step.ScenarioID,
								Order:            step.Order,
								Title:            step.Title,
								TextContent:      step.TextContent,
								HintContent:      step.HintContent,
								HasFlag:          step.HasFlag,
								FlagLevel:        step.FlagLevel,
								CreatedAt:        step.CreatedAt,
								UpdatedAt:        step.UpdatedAt,
							})
						}
						output.Steps = steps
					}

					return output, nil
				},
				DtoToModel: func(input dto.CreateScenarioInput) *models.Scenario {
					scenario := &models.Scenario{
						Name:           input.Name,
						Title:          input.Title,
						Description:    input.Description,
						Difficulty:     input.Difficulty,
						EstimatedTime:  input.EstimatedTime,
						InstanceType:   input.InstanceType,
						SourceType:     input.SourceType,
						GitRepository:  input.GitRepository,
						GitBranch:      input.GitBranch,
						SourcePath:     input.SourcePath,
						FlagsEnabled:   input.FlagsEnabled,
						GshEnabled:     input.GshEnabled,
						CrashTraps:     input.CrashTraps,
						IntroText:      input.IntroText,
						FinishText:     input.FinishText,
						OrganizationID: input.OrganizationID,
					}
					return scenario
				},
				DtoToMap: func(input dto.EditScenarioInput) map[string]any {
					updates := make(map[string]any)
					if input.Name != nil {
						updates["name"] = *input.Name
					}
					if input.Title != nil {
						updates["title"] = *input.Title
					}
					if input.Description != nil {
						updates["description"] = *input.Description
					}
					if input.Difficulty != nil {
						updates["difficulty"] = *input.Difficulty
					}
					if input.EstimatedTime != nil {
						updates["estimated_time"] = *input.EstimatedTime
					}
					if input.InstanceType != nil {
						updates["instance_type"] = *input.InstanceType
					}
					if input.SourceType != nil {
						updates["source_type"] = *input.SourceType
					}
					if input.GitRepository != nil {
						updates["git_repository"] = *input.GitRepository
					}
					if input.GitBranch != nil {
						updates["git_branch"] = *input.GitBranch
					}
					if input.SourcePath != nil {
						updates["source_path"] = *input.SourcePath
					}
					if input.FlagsEnabled != nil {
						updates["flags_enabled"] = *input.FlagsEnabled
					}
					if input.GshEnabled != nil {
						updates["gsh_enabled"] = *input.GshEnabled
					}
					if input.CrashTraps != nil {
						updates["crash_traps"] = *input.CrashTraps
					}
					if input.IntroText != nil {
						updates["intro_text"] = *input.IntroText
					}
					if input.FinishText != nil {
						updates["finish_text"] = *input.FinishText
					}
					if input.OrganizationID != nil {
						updates["organization_id"] = *input.OrganizationID
					}
					return updates
				},
			},
			SubEntities: []any{models.ScenarioStep{}},
			DefaultIncludes: []string{"Steps"},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "scenarios",
				EntityName: "Scenario",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "List all scenarios",
					Description: "Retrieve all available scenarios",
					Tags:        []string{"scenarios"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get a scenario",
					Description: "Retrieve a specific scenario by ID",
					Tags:        []string{"scenarios"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create a scenario",
					Description: "Create a new interactive lab scenario",
					Tags:        []string{"scenarios"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update a scenario",
					Description: "Update an existing scenario",
					Tags:        []string{"scenarios"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete a scenario",
					Description: "Delete a scenario and all its steps",
					Tags:        []string{"scenarios"},
					Security:    true,
				},
			},
		},
	)
}
