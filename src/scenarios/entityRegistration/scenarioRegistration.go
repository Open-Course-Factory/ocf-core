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
						Hostname:       model.Hostname,
						OsType:         model.OsType,
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
						SetupScriptID:  model.SetupScriptID,
						IntroFileID:    model.IntroFileID,
						FinishFileID:   model.FinishFileID,
						CreatedAt:      model.CreatedAt,
						UpdatedAt:      model.UpdatedAt,
					}

					if len(model.Steps) > 0 {
						steps := make([]dto.ScenarioStepOutput, 0, len(model.Steps))
						for _, step := range model.Steps {
							steps = append(steps, dto.ScenarioStepOutput{
								ID:                 step.ID,
								ScenarioID:         step.ScenarioID,
								Order:              step.Order,
								Title:              step.Title,
								TextContent:        step.TextContent,
								HintContent:        step.HintContent,
								HasFlag:            step.HasFlag,
								FlagPath:           step.FlagPath,
								FlagLevel:          step.FlagLevel,
								VerifyScriptID:     step.VerifyScriptID,
								BackgroundScriptID: step.BackgroundScriptID,
								ForegroundScriptID: step.ForegroundScriptID,
								TextFileID:         step.TextFileID,
								HintFileID:         step.HintFileID,
								CreatedAt:          step.CreatedAt,
								UpdatedAt:          step.UpdatedAt,
							})
						}
						output.Steps = steps
					}

					if len(model.CompatibleInstanceTypes) > 0 {
						types := make([]dto.ScenarioInstanceTypeOutput, 0, len(model.CompatibleInstanceTypes))
						for _, t := range model.CompatibleInstanceTypes {
							types = append(types, dto.ScenarioInstanceTypeOutput{
								ID:           t.ID,
								ScenarioID:   t.ScenarioID,
								InstanceType: t.InstanceType,
								OsType:       t.OsType,
								Priority:     t.Priority,
								CreatedAt:    t.CreatedAt,
								UpdatedAt:    t.UpdatedAt,
							})
						}
						output.CompatibleInstanceTypes = types
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
						Hostname:       input.Hostname,
						OsType:         input.OsType,
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
						SetupScriptID:  input.SetupScriptID,
						IntroFileID:    input.IntroFileID,
						FinishFileID:   input.FinishFileID,
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
					if input.Hostname != nil {
						updates["hostname"] = *input.Hostname
					}
					if input.OsType != nil {
						updates["os_type"] = *input.OsType
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
					if input.SetupScriptID != nil {
						updates["setup_script_id"] = *input.SetupScriptID
					}
					if input.IntroFileID != nil {
						updates["intro_file_id"] = *input.IntroFileID
					}
					if input.FinishFileID != nil {
						updates["finish_file_id"] = *input.FinishFileID
					}
					return updates
				},
			},
			SubEntities: []any{models.ScenarioStep{}, models.ScenarioInstanceType{}},
			DefaultIncludes: []string{"Steps", "CompatibleInstanceTypes"},
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
