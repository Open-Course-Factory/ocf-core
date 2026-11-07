package scenarioRegistration

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

func RegisterScenarioAssignment(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ScenarioAssignment, dto.CreateScenarioAssignmentInput, dto.EditScenarioAssignmentInput, dto.ScenarioAssignmentOutput](
		service,
		"ScenarioAssignment",
		entityManagementInterfaces.TypedEntityRegistration[models.ScenarioAssignment, dto.CreateScenarioAssignmentInput, dto.EditScenarioAssignmentInput, dto.ScenarioAssignmentOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ScenarioAssignment, dto.CreateScenarioAssignmentInput, dto.EditScenarioAssignmentInput, dto.ScenarioAssignmentOutput]{
				ModelToDto: func(model *models.ScenarioAssignment) (dto.ScenarioAssignmentOutput, error) {
					return ScenarioAssignmentToOutput(model), nil
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
	service.RegisterListScope("ScenarioAssignment", scenarioAssignmentListScope)
	service.RegisterDtoListEnricher("ScenarioAssignment", EnrichScenarioAssignmentOutputs)
}

// ScenarioAssignmentToOutput is an assignment as the entity routes return it,
// before EnrichScenarioAssignmentOutputs adds the derived fields.
func ScenarioAssignmentToOutput(model *models.ScenarioAssignment) dto.ScenarioAssignmentOutput {
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
		ResultsPublishedAt: model.ResultsPublishedAt,
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
			EstimatedTimeMinutes:  model.Scenario.EstimatedTimeMinutes,
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
	return output
}

// EnrichScenarioAssignmentOutputs sets HasExamQuiz on a page of assignments in
// one query, with the exam-scenario lookup grade withholding uses.
func EnrichScenarioAssignmentOutputs(_ *gin.Context, dtos []any, db *gorm.DB) error {
	scenarioIDs := make([]uuid.UUID, 0, len(dtos))
	for _, d := range dtos {
		if out, ok := d.(dto.ScenarioAssignmentOutput); ok {
			scenarioIDs = append(scenarioIDs, out.ScenarioID)
		}
	}
	exam, err := services.ExamScenarios(db, scenarioIDs)
	if err != nil {
		return err
	}
	for i, d := range dtos {
		if out, ok := d.(dto.ScenarioAssignmentOutput); ok {
			out.HasExamQuiz = exam[out.ScenarioID]
			dtos[i] = out
		}
	}
	return nil
}
