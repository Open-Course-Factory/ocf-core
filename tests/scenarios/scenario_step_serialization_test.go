package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	"soli/formations/src/scenarios/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
)

// setupScenarioRegistrationService creates a fresh EntityRegistrationService with
// Scenario, ScenarioStep and ScenarioStepQuestion registered. It exercises the
// real registration converters used in production HTTP handlers.
func setupScenarioRegistrationService(t *testing.T) *ems.EntityRegistrationService {
	t.Helper()
	svc := ems.NewEntityRegistrationService()
	scenarioRegistration.RegisterScenario(svc)
	scenarioRegistration.RegisterScenarioStep(svc)
	scenarioRegistration.RegisterScenarioStepQuestion(svc)
	return svc
}

// TestScenarioOutput_WithIncludeSteps_PreservesStepType verifies that when a
// scenario has a quiz step in its preloaded Steps slice, the ModelToDto
// converter copies StepType into ScenarioStepOutput. Without this, every step
// reloaded by the editor looks like a terminal step.
func TestScenarioOutput_WithIncludeSteps_PreservesStepType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "preserve-steptype", Title: "Preserve StepType", InstanceType: "ubuntu:22.04", CreatedByID: "u1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Quiz Step", StepType: "quiz",
	}
	require.NoError(t, db.Create(&step).Error)

	// Reload scenario with preloaded steps (mirrors ?include=steps behavior)
	var loaded models.Scenario
	require.NoError(t, db.Preload("Steps").First(&loaded, "id = ?", scenario.ID).Error)
	require.Len(t, loaded.Steps, 1)

	svc := setupScenarioRegistrationService(t)
	ops, ok := svc.GetEntityOps("Scenario")
	require.True(t, ok, "Scenario must be registered")

	rawDto, err := ops.ConvertModelToDto(&loaded)
	require.NoError(t, err)

	output, ok := rawDto.(dto.ScenarioOutput)
	require.True(t, ok, "output must be a ScenarioOutput")

	require.Len(t, output.Steps, 1, "scenario output must contain the preloaded step")
	assert.Equal(t, "quiz", output.Steps[0].StepType, "step_type must be preserved through ScenarioOutput serialization")
}

// TestScenarioOutput_WithIncludeStepsQuestions_PreloadsQuestions verifies that
// a scenario reloaded with Steps.Questions preloaded carries questions through
// the ModelToDto converter. This is the round-trip the trainer experiences
// when saving a quiz step and reloading the scenario.
func TestScenarioOutput_WithIncludeStepsQuestions_PreloadsQuestions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "preload-questions", Title: "Preload Questions", InstanceType: "ubuntu:22.04", CreatedByID: "u1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Quiz Step", StepType: "quiz",
	}
	require.NoError(t, db.Create(&step).Error)

	q1 := models.ScenarioStepQuestion{
		StepID:        step.ID,
		Order:         1,
		QuestionText:  "What command lists files?",
		QuestionType:  "multiple_choice",
		Options:       `["ls", "cd", "rm", "cat"]`,
		CorrectAnswer: "ls",
		Explanation:   "ls lists directory contents",
		Points:        2,
	}
	q2 := models.ScenarioStepQuestion{
		StepID:        step.ID,
		Order:         2,
		QuestionText:  "True or false: /dev/null discards data",
		QuestionType:  "true_false",
		CorrectAnswer: "true",
		Points:        1,
	}
	require.NoError(t, db.Create(&q1).Error)
	require.NoError(t, db.Create(&q2).Error)

	// Reload scenario with Steps.Questions preloaded (mirrors DefaultIncludes:["Steps.Questions"])
	var loaded models.Scenario
	require.NoError(t, db.Preload("Steps.Questions").First(&loaded, "id = ?", scenario.ID).Error)
	require.Len(t, loaded.Steps, 1)
	require.Len(t, loaded.Steps[0].Questions, 2, "preloaded scenario must have 2 questions on the step")

	svc := setupScenarioRegistrationService(t)
	ops, ok := svc.GetEntityOps("Scenario")
	require.True(t, ok, "Scenario must be registered")

	rawDto, err := ops.ConvertModelToDto(&loaded)
	require.NoError(t, err)

	output, ok := rawDto.(dto.ScenarioOutput)
	require.True(t, ok, "output must be a ScenarioOutput")

	require.Len(t, output.Steps, 1)
	assert.Equal(t, "quiz", output.Steps[0].StepType)

	require.Len(t, output.Steps[0].Questions, 2, "ScenarioOutput.Steps[0].Questions must contain both preloaded questions")

	// Find the multiple-choice question by question_text (order may vary depending on DB)
	var mc *dto.ScenarioStepQuestionOutput
	for i := range output.Steps[0].Questions {
		if output.Steps[0].Questions[i].QuestionType == "multiple_choice" {
			mc = &output.Steps[0].Questions[i]
			break
		}
	}
	require.NotNil(t, mc, "multiple_choice question must be present in output")
	assert.Equal(t, "What command lists files?", mc.QuestionText)
	assert.Equal(t, `["ls", "cd", "rm", "cat"]`, mc.Options)
	assert.Equal(t, "ls", mc.CorrectAnswer)
	assert.Equal(t, 2, mc.Points)
}

// TestScenarioStepOutput_GetSingle_IncludesQuestions verifies that requesting a
// single ScenarioStep returns its questions through the registered ModelToDto.
// The DefaultIncludes:["Questions"] on ScenarioStep registration ensures GORM
// preloads the relation so the converter has data to serialize.
func TestScenarioStepOutput_GetSingle_IncludesQuestions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "step-single", Title: "Single Step", InstanceType: "ubuntu:22.04", CreatedByID: "u1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Quiz Step", StepType: "quiz",
	}
	require.NoError(t, db.Create(&step).Error)

	require.NoError(t, db.Create(&models.ScenarioStepQuestion{
		StepID:        step.ID,
		Order:         1,
		QuestionText:  "First?",
		QuestionType:  "free_text",
		CorrectAnswer: "yes",
		Points:        3,
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepQuestion{
		StepID:        step.ID,
		Order:         2,
		QuestionText:  "Second?",
		QuestionType:  "free_text",
		CorrectAnswer: "no",
		Points:        1,
	}).Error)

	// Mirror the GET /scenario-steps/:id flow: GORM preloads Questions
	// (via DefaultIncludes) before ModelToDto is called.
	var loaded models.ScenarioStep
	require.NoError(t, db.Preload("Questions").First(&loaded, "id = ?", step.ID).Error)
	require.Len(t, loaded.Questions, 2)

	svc := setupScenarioRegistrationService(t)
	ops, ok := svc.GetEntityOps("ScenarioStep")
	require.True(t, ok, "ScenarioStep must be registered")

	rawDto, err := ops.ConvertModelToDto(&loaded)
	require.NoError(t, err)

	output, ok := rawDto.(dto.ScenarioStepOutput)
	require.True(t, ok, "output must be a ScenarioStepOutput")

	assert.Equal(t, "quiz", output.StepType)
	require.Len(t, output.Questions, 2, "ScenarioStepOutput.Questions must contain preloaded questions")

	// Verify the registered DefaultIncludes carries Questions for the Steps
	// preload chain. This is what makes the GET /scenarios/:id?include=steps
	// path return questions out of the box.
	scenarioIncludes := svc.GetDefaultIncludes("Scenario")
	assert.Contains(t, scenarioIncludes, "Steps.Questions",
		"Scenario DefaultIncludes must include Steps.Questions so questions reload by default")

	stepIncludes := svc.GetDefaultIncludes("ScenarioStep")
	assert.Contains(t, stepIncludes, "Questions",
		"ScenarioStep DefaultIncludes must include Questions so a single step GET reloads them")
}
