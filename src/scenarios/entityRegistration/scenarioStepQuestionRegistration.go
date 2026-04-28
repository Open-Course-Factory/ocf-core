package scenarioRegistration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func RegisterScenarioStepQuestion(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioStepQuestion, dto.CreateScenarioStepQuestionInput, dto.EditScenarioStepQuestionInput, dto.ScenarioStepQuestionOutput](
		service,
		"ScenarioStepQuestion",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioStepQuestion, dto.CreateScenarioStepQuestionInput, dto.EditScenarioStepQuestionInput, dto.ScenarioStepQuestionOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioStepQuestion, dto.CreateScenarioStepQuestionInput, dto.EditScenarioStepQuestionInput, dto.ScenarioStepQuestionOutput]{
				ModelToDto: func(model *models.ScenarioStepQuestion) (dto.ScenarioStepQuestionOutput, error) {
					return dto.ScenarioStepQuestionOutput{
						ID:            model.ID,
						StepID:        model.StepID,
						Order:         model.Order,
						QuestionText:  model.QuestionText,
						QuestionType:  model.QuestionType,
						Options:       model.Options,
						CorrectAnswer: model.CorrectAnswer,
						Explanation:   model.Explanation,
						Points:        model.Points,
						CreatedAt:     model.CreatedAt,
						UpdatedAt:     model.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateScenarioStepQuestionInput) *models.ScenarioStepQuestion {
					points := input.Points
					if points == 0 {
						points = 1
					}
					return &models.ScenarioStepQuestion{
						StepID:        input.StepID,
						Order:         input.Order,
						QuestionText:  input.QuestionText,
						QuestionType:  input.QuestionType,
						Options:       input.Options,
						CorrectAnswer: input.CorrectAnswer,
						Explanation:   input.Explanation,
						Points:        points,
					}
				},
				DtoToMap: func(input dto.EditScenarioStepQuestionInput) map[string]any {
					updates := make(map[string]any)
					if input.Order != nil {
						updates["order"] = *input.Order
					}
					if input.QuestionText != nil {
						updates["question_text"] = *input.QuestionText
					}
					if input.QuestionType != nil {
						updates["question_type"] = *input.QuestionType
					}
					if input.Options != nil {
						updates["options"] = *input.Options
					}
					if input.CorrectAnswer != nil {
						updates["correct_answer"] = *input.CorrectAnswer
					}
					if input.Explanation != nil {
						updates["explanation"] = *input.Explanation
					}
					if input.Points != nil {
						updates["points"] = *input.Points
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
				Tag:        "scenario-step-questions",
				EntityName: "ScenarioStepQuestion",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "List all scenario step questions",
					Description: "Retrieve all quiz questions for scenario steps",
					Tags:        []string{"scenario-step-questions"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get a scenario step question",
					Description: "Retrieve a specific scenario step question by ID",
					Tags:        []string{"scenario-step-questions"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create a scenario step question",
					Description: "Create a new quiz question for a scenario step",
					Tags:        []string{"scenario-step-questions"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update a scenario step question",
					Description: "Update an existing scenario step question",
					Tags:        []string{"scenario-step-questions"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete a scenario step question",
					Description: "Delete a scenario step question (admin only)",
					Tags:        []string{"scenario-step-questions"},
					Security:    true,
				},
			},
		},
	)
}
