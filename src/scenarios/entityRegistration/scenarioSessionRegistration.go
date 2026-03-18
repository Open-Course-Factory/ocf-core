package scenarioRegistration

import (
	"net/http"
	"time"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func RegisterScenarioSession(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioSession, dto.CreateScenarioSessionInput, dto.EditScenarioSessionInput, dto.ScenarioSessionOutput](
		service,
		"ScenarioSession",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioSession, dto.CreateScenarioSessionInput, dto.EditScenarioSessionInput, dto.ScenarioSessionOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioSession, dto.CreateScenarioSessionInput, dto.EditScenarioSessionInput, dto.ScenarioSessionOutput]{
				ModelToDto: func(model *models.ScenarioSession) (dto.ScenarioSessionOutput, error) {
					output := dto.ScenarioSessionOutput{
						ID:                model.ID,
						ScenarioID:        model.ScenarioID,
						UserID:            model.UserID,
						TerminalSessionID: model.TerminalSessionID,
						CurrentStep:       model.CurrentStep,
						Status:            model.Status,
						StartedAt:         model.StartedAt,
						CompletedAt:       model.CompletedAt,
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
				DtoToModel: func(input dto.CreateScenarioSessionInput) *models.ScenarioSession {
					return &models.ScenarioSession{
						ScenarioID:        input.ScenarioID,
						UserID:            input.UserID,
						TerminalSessionID: input.TerminalSessionID,
						Status:            "active",
						StartedAt:         time.Now(),
					}
				},
				DtoToMap: func(input dto.EditScenarioSessionInput) map[string]any {
					updates := make(map[string]any)
					if input.TerminalSessionID != nil {
						updates["terminal_session_id"] = *input.TerminalSessionID
					}
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
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
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
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Start a scenario session",
					Description: "Start a new scenario session for the current user",
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
