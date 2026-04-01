package scenarioRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func RegisterScenarioAssignment(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioAssignment, dto.CreateScenarioAssignmentInput, dto.EditScenarioAssignmentInput, dto.ScenarioAssignmentOutput](
		service,
		"ScenarioAssignment",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioAssignment, dto.CreateScenarioAssignmentInput, dto.EditScenarioAssignmentInput, dto.ScenarioAssignmentOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioAssignment, dto.CreateScenarioAssignmentInput, dto.EditScenarioAssignmentInput, dto.ScenarioAssignmentOutput]{
				ModelToDto: func(model *models.ScenarioAssignment) (dto.ScenarioAssignmentOutput, error) {
					output := dto.ScenarioAssignmentOutput{
						ID:             model.ID,
						ScenarioID:     model.ScenarioID,
						GroupID:        model.GroupID,
						OrganizationID: model.OrganizationID,
						Scope:          model.Scope,
						CreatedByID:    model.CreatedByID,
						StartDate:      model.StartDate,
						Deadline:       model.Deadline,
						IsActive:       model.IsActive,
						CreatedAt:      model.CreatedAt,
						UpdatedAt:      model.UpdatedAt,
					}

					if model.Scenario.ID.String() != "00000000-0000-0000-0000-000000000000" {
						scenarioOutput := dto.ScenarioOutput{
							ID:             model.Scenario.ID,
							Name:           model.Scenario.Name,
							Title:          model.Scenario.Title,
							Description:    model.Scenario.Description,
							Difficulty:     model.Scenario.Difficulty,
							EstimatedTime:  model.Scenario.EstimatedTime,
							InstanceType:   model.Scenario.InstanceType,
							OsType:         model.Scenario.OsType,
							SourceType:     model.Scenario.SourceType,
							CreatedByID:    model.Scenario.CreatedByID,
							OrganizationID: model.Scenario.OrganizationID,
							CreatedAt:      model.Scenario.CreatedAt,
							UpdatedAt:      model.Scenario.UpdatedAt,
						}
						output.Scenario = &scenarioOutput
					}

					return output, nil
				},
				DtoToModel: func(input dto.CreateScenarioAssignmentInput) *models.ScenarioAssignment {
					return &models.ScenarioAssignment{
						ScenarioID:     input.ScenarioID,
						GroupID:        input.GroupID,
						OrganizationID: input.OrganizationID,
						Scope:          input.Scope,
						StartDate:      input.StartDate,
						Deadline:       input.Deadline,
						IsActive:       input.IsActive,
					}
				},
				DtoToMap: func(input dto.EditScenarioAssignmentInput) map[string]any {
					updates := make(map[string]any)
					if input.StartDate != nil {
						updates["start_date"] = *input.StartDate
					}
					if input.Deadline != nil {
						updates["deadline"] = *input.Deadline
					}
					if input.IsActive != nil {
						updates["is_active"] = *input.IsActive
					}
					return updates
				},
			},
			DefaultIncludes: []string{"Scenario"},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "scenario-assignments",
				EntityName: "ScenarioAssignment",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "List all scenario assignments",
					Description: "Retrieve all scenario assignments",
					Tags:        []string{"scenario-assignments"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get a scenario assignment",
					Description: "Retrieve a specific scenario assignment by ID",
					Tags:        []string{"scenario-assignments"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create a scenario assignment",
					Description: "Assign a scenario to a group or organization",
					Tags:        []string{"scenario-assignments"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update a scenario assignment",
					Description: "Update an existing scenario assignment",
					Tags:        []string{"scenario-assignments"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete a scenario assignment",
					Description: "Delete a scenario assignment",
					Tags:        []string{"scenario-assignments"},
					Security:    true,
				},
			},
		},
	)
}
