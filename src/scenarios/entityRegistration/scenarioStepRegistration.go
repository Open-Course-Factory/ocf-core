package scenarioRegistration

import (
	"net/http"
	"sort"

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
					output := dto.ScenarioStepOutput{
						ID:                 model.ID,
						ScenarioID:         model.ScenarioID,
						Order:              model.Order,
						Title:              model.Title,
						StepType:           model.StepType,
						ShowImmediateFeedback: model.ShowImmediateFeedback,
						TextContent:        model.TextContent,
						HintContent:        model.HintContent,
						VerifyScript:       model.VerifyScript,
						BackgroundScript:   model.BackgroundScript,
						ForegroundScript:   model.ForegroundScript,
						HasFlag:            model.HasFlag,
						FlagPath:           model.FlagPath,
						FlagLevel:          model.FlagLevel,
						VerifyScriptID:     model.VerifyScriptID,
						BackgroundScriptID: model.BackgroundScriptID,
						ForegroundScriptID: model.ForegroundScriptID,
						TextFileID:         model.TextFileID,
						HintFileID:         model.HintFileID,
						CreatedAt:          model.CreatedAt,
						UpdatedAt:          model.UpdatedAt,
					}
					if len(model.Questions) > 0 {
						// GORM's Preload doesn't apply ordering by default —
						// sort here so the editor and player always see
						// questions in author-defined order.
						sortedQuestions := make([]models.ScenarioStepQuestion, len(model.Questions))
						copy(sortedQuestions, model.Questions)
						sort.SliceStable(sortedQuestions, func(i, j int) bool {
							return sortedQuestions[i].Order < sortedQuestions[j].Order
						})
						questions := make([]dto.ScenarioStepQuestionOutput, 0, len(sortedQuestions))
						for _, q := range sortedQuestions {
							questions = append(questions, dto.ScenarioStepQuestionOutput{
								ID:            q.ID,
								StepID:        q.StepID,
								Order:         q.Order,
								QuestionText:  q.QuestionText,
								QuestionType:  q.QuestionType,
								Options:       q.Options,
								CorrectAnswer: q.CorrectAnswer,
								Explanation:   q.Explanation,
								Points:        q.Points,
								CreatedAt:     q.CreatedAt,
								UpdatedAt:     q.UpdatedAt,
							})
						}
						output.Questions = questions
					}
					return output, nil
				},
				DtoToModel: func(input dto.CreateScenarioStepInput) *models.ScenarioStep {
					stepType := input.StepType
					if stepType == "" {
						stepType = "terminal"
					}
					return &models.ScenarioStep{
						ScenarioID:         input.ScenarioID,
						Order:              input.Order,
						Title:              input.Title,
						StepType:           stepType,
						ShowImmediateFeedback: input.ShowImmediateFeedback,
						TextContent:        input.TextContent,
						HintContent:        input.HintContent,
						VerifyScript:       input.VerifyScript,
						BackgroundScript:   input.BackgroundScript,
						ForegroundScript:   input.ForegroundScript,
						HasFlag:            input.HasFlag,
						FlagPath:           input.FlagPath,
						FlagLevel:          input.FlagLevel,
						VerifyScriptID:     input.VerifyScriptID,
						BackgroundScriptID: input.BackgroundScriptID,
						ForegroundScriptID: input.ForegroundScriptID,
						TextFileID:         input.TextFileID,
						HintFileID:         input.HintFileID,
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
					if input.StepType != nil {
						updates["step_type"] = *input.StepType
					}
					if input.ShowImmediateFeedback != nil {
						updates["show_immediate_feedback"] = *input.ShowImmediateFeedback
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
					if input.VerifyScriptID != nil {
						updates["verify_script_id"] = *input.VerifyScriptID
					}
					if input.BackgroundScriptID != nil {
						updates["background_script_id"] = *input.BackgroundScriptID
					}
					if input.ForegroundScriptID != nil {
						updates["foreground_script_id"] = *input.ForegroundScriptID
					}
					if input.TextFileID != nil {
						updates["text_file_id"] = *input.TextFileID
					}
					if input.HintFileID != nil {
						updates["hint_file_id"] = *input.HintFileID
					}
					return updates
				},
			},
			DefaultIncludes: []string{"Questions"},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					// Members may CRUD steps; the ScenarioStepAuthorizationHook
					// gates write operations to scenarios the user can manage
					// (creator / org-manager / group-manager via assignment).
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
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
