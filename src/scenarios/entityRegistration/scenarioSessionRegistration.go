package scenarioRegistration

import (
	"net/http"

	access "soli/formations/src/auth/access"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func RegisterScenarioSession(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioSession, struct{}, dto.EditScenarioSessionInput, dto.ScenarioSessionOutput](
		service,
		"ScenarioSession",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioSession, struct{}, dto.EditScenarioSessionInput, dto.ScenarioSessionOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioSession, struct{}, dto.EditScenarioSessionInput, dto.ScenarioSessionOutput]{
				ModelToDto: func(model *models.ScenarioSession) (dto.ScenarioSessionOutput, error) {
					output := dto.ScenarioSessionOutput{
						ID:                model.ID,
						ScenarioID:        model.ScenarioID,
						UserID:            model.UserID,
						TerminalSessionID: model.TerminalSessionID,
						CurrentStep:       model.CurrentStep,
						Status:            model.Status,
						ProvisioningPhase: model.ProvisioningPhase,
						StartedAt:         model.StartedAt,
						CompletedAt:       model.CompletedAt,
						IsPreview:         model.IsPreview,
						CreatedAt:         model.CreatedAt,
						UpdatedAt:         model.UpdatedAt,
					}

					if len(model.StepProgress) > 0 {
						progress := make([]dto.ScenarioStepProgressOutput, 0, len(model.StepProgress))
						for _, p := range model.StepProgress {
							progress = append(progress, dto.ScenarioStepProgressOutput{
								ID:               p.ID,
								SessionID:        p.SessionID,
								StepOrder:        p.StepOrder,
								Status:           p.Status,
								VerifyAttempts:   p.VerifyAttempts,
								CompletedAt:      p.CompletedAt,
								TimeSpentSeconds: p.TimeSpentSeconds,
								CreatedAt:        p.CreatedAt,
								UpdatedAt:        p.UpdatedAt,
							})
						}
						output.StepProgress = progress
					}

					if len(model.Flags) > 0 {
						flags := make([]dto.ScenarioFlagOutput, 0, len(model.Flags))
						for _, f := range model.Flags {
							flags = append(flags, dto.ScenarioFlagOutput{
								ID:            f.ID,
								SessionID:     f.SessionID,
								StepOrder:     f.StepOrder,
								SubmittedFlag: f.SubmittedFlag,
								SubmittedAt:   f.SubmittedAt,
								IsCorrect:     f.IsCorrect,
								CreatedAt:     f.CreatedAt,
								UpdatedAt:     f.UpdatedAt,
							})
						}
						output.Flags = flags
					}

					return output, nil
				},
				// No generic create: there is no input to map, so a Create op added
				// later cannot accept a client-chosen owner or terminal.
				DtoToModel: func(struct{}) *models.ScenarioSession { return &models.ScenarioSession{} },
				DtoToMap: func(input dto.EditScenarioSessionInput) map[string]any {
					updates := make(map[string]any)
					if input.CurrentStep != nil {
						updates["current_step"] = *input.CurrentStep
					}
					if input.Status != nil {
						updates["status"] = *input.Status
					}
					if input.CompletedAt != nil {
						updates["completed_at"] = *input.CompletedAt
					}
					return updates
				},
			},
			SubEntities:     []any{models.ScenarioStepProgress{}, models.ScenarioFlag{}},
			DefaultIncludes: []string{"StepProgress", "Flags"},
			OwnershipConfig: &access.OwnershipConfig{OwnerField: "UserID", Operations: []string{"read"}, AdminBypass: true},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					// No generic create: a run is created by the launch routes, which run the access and terminal checks.
					string(authModels.Member): "(" + http.MethodGet + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "scenario-sessions",
				EntityName: "ScenarioSession",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "List all scenario sessions",
					Description: "Retrieve all scenario sessions",
					Tags:        []string{"scenario-sessions"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get a scenario session",
					Description: "Retrieve a specific scenario session by ID",
					Tags:        []string{"scenario-sessions"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update a scenario session",
					Description: "Update scenario session progress",
					Tags:        []string{"scenario-sessions"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete a scenario session",
					Description: "Delete a scenario session (admin only)",
					Tags:        []string{"scenario-sessions"},
					Security:    true,
				},
			},
		},
	)
}
