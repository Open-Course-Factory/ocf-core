package scenarioRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func RegisterScenarioInstanceType(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioInstanceType, dto.CreateScenarioInstanceTypeInput, dto.EditScenarioInstanceTypeInput, dto.ScenarioInstanceTypeOutput](
		service,
		"ScenarioInstanceType",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioInstanceType, dto.CreateScenarioInstanceTypeInput, dto.EditScenarioInstanceTypeInput, dto.ScenarioInstanceTypeOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioInstanceType, dto.CreateScenarioInstanceTypeInput, dto.EditScenarioInstanceTypeInput, dto.ScenarioInstanceTypeOutput]{
				ModelToDto: func(model *models.ScenarioInstanceType) (dto.ScenarioInstanceTypeOutput, error) {
					return dto.ScenarioInstanceTypeOutput{
						ID:           model.ID,
						ScenarioID:   model.ScenarioID,
						InstanceType: model.InstanceType,
						OsType:       model.OsType,
						Priority:     model.Priority,
						CreatedAt:    model.CreatedAt,
						UpdatedAt:    model.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateScenarioInstanceTypeInput) *models.ScenarioInstanceType {
					return &models.ScenarioInstanceType{
						ScenarioID:   input.ScenarioID,
						InstanceType: input.InstanceType,
						OsType:       input.OsType,
						Priority:     input.Priority,
					}
				},
				DtoToMap: func(input dto.EditScenarioInstanceTypeInput) map[string]any {
					updates := make(map[string]any)
					if input.InstanceType != nil {
						updates["instance_type"] = *input.InstanceType
					}
					if input.OsType != nil {
						updates["os_type"] = *input.OsType
					}
					if input.Priority != nil {
						updates["priority"] = *input.Priority
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
				Tag:        "scenario-instance-types",
				EntityName: "ScenarioInstanceType",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "List all scenario instance types",
					Description: "Retrieve all compatible instance types for scenarios",
					Tags:        []string{"scenario-instance-types"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get a scenario instance type",
					Description: "Retrieve a specific scenario instance type by ID",
					Tags:        []string{"scenario-instance-types"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create a scenario instance type",
					Description: "Add a new compatible instance type to a scenario",
					Tags:        []string{"scenario-instance-types"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update a scenario instance type",
					Description: "Update an existing scenario instance type",
					Tags:        []string{"scenario-instance-types"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete a scenario instance type",
					Description: "Delete a scenario instance type",
					Tags:        []string{"scenario-instance-types"},
					Security:    true,
				},
			},
		},
	)
}
