package scenarioRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func RegisterScenarioStepProgress(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioStepProgress, dto.CreateScenarioStepProgressInput, dto.EditScenarioStepProgressInput, dto.ScenarioStepProgressOutput](
		service,
		"ScenarioStepProgress",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioStepProgress, dto.CreateScenarioStepProgressInput, dto.EditScenarioStepProgressInput, dto.ScenarioStepProgressOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioStepProgress, dto.CreateScenarioStepProgressInput, dto.EditScenarioStepProgressInput, dto.ScenarioStepProgressOutput]{
				ModelToDto: func(model *models.ScenarioStepProgress) (dto.ScenarioStepProgressOutput, error) {
					return dto.ScenarioStepProgressOutput{
						ID:               model.ID,
						SessionID:        model.SessionID,
						StepOrder:        model.StepOrder,
						Status:           model.Status,
						VerifyAttempts:   model.VerifyAttempts,
						CompletedAt:      model.CompletedAt,
						TimeSpentSeconds: model.TimeSpentSeconds,
						CreatedAt:        model.CreatedAt,
						UpdatedAt:        model.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateScenarioStepProgressInput) *models.ScenarioStepProgress {
					status := input.Status
					if status == "" {
						status = "locked"
					}
					return &models.ScenarioStepProgress{
						SessionID: input.SessionID,
						StepOrder: input.StepOrder,
						Status:    status,
					}
				},
				DtoToMap: func(input dto.EditScenarioStepProgressInput) map[string]any {
					updates := make(map[string]any)
					if input.Status != nil {
						updates["status"] = *input.Status
					}
					if input.VerifyAttempts != nil {
						updates["verify_attempts"] = *input.VerifyAttempts
					}
					if input.CompletedAt != nil {
						updates["completed_at"] = *input.CompletedAt
					}
					if input.TimeSpentSeconds != nil {
						updates["time_spent_seconds"] = *input.TimeSpentSeconds
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
				Tag:        "scenario-step-progress",
				EntityName: "ScenarioStepProgress",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "List all step progress records",
					Description: "Retrieve all scenario step progress entries",
					Tags:        []string{"scenario-step-progress"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get step progress",
					Description: "Retrieve a specific step progress entry by ID",
					Tags:        []string{"scenario-step-progress"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create step progress",
					Description: "Create a new step progress entry for a session",
					Tags:        []string{"scenario-step-progress"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update step progress",
					Description: "Update step progress (status, attempts, time spent)",
					Tags:        []string{"scenario-step-progress"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete step progress",
					Description: "Delete a step progress entry (admin only)",
					Tags:        []string{"scenario-step-progress"},
					Security:    true,
				},
			},
		},
	)
}
