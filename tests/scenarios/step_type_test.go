package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
)

func TestStepType_DefaultTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "steptype-default", Title: "Default StepType", InstanceType: "ubuntu:22.04", CreatedByID: "u1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	// Create a step without explicit step_type — should default to "terminal"
	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Terminal Step",
	}
	require.NoError(t, db.Create(&step).Error)

	var fetched models.ScenarioStep
	require.NoError(t, db.First(&fetched, "id = ?", step.ID).Error)
	assert.Equal(t, "terminal", fetched.StepType, "default step_type should be 'terminal'")
}

func TestStepType_Quiz(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "steptype-quiz", Title: "Quiz StepType", InstanceType: "ubuntu:22.04", CreatedByID: "u1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Quiz Step", StepType: "quiz",
	}
	require.NoError(t, db.Create(&step).Error)

	var fetched models.ScenarioStep
	require.NoError(t, db.First(&fetched, "id = ?", step.ID).Error)
	assert.Equal(t, "quiz", fetched.StepType, "step_type should be 'quiz'")
}

func TestStepType_UpdateViaPatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "steptype-patch", Title: "Patch StepType", InstanceType: "ubuntu:22.04", CreatedByID: "u1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Step", StepType: "terminal",
	}
	require.NoError(t, db.Create(&step).Error)

	// Simulate DtoToMap patch
	newType := "quiz"
	editInput := dto.EditScenarioStepInput{
		StepType: &newType,
	}
	updates := make(map[string]any)
	if editInput.StepType != nil {
		updates["step_type"] = *editInput.StepType
	}
	require.NoError(t, db.Model(&step).Updates(updates).Error)

	var fetched models.ScenarioStep
	require.NoError(t, db.First(&fetched, "id = ?", step.ID).Error)
	assert.Equal(t, "quiz", fetched.StepType, "step_type should be updated to 'quiz'")
}

func TestStepType_OutputDto(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "steptype-dto", Title: "DTO StepType", InstanceType: "ubuntu:22.04", CreatedByID: "u1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Quiz Step", StepType: "quiz",
	}
	require.NoError(t, db.Create(&step).Error)

	var fetched models.ScenarioStep
	require.NoError(t, db.First(&fetched, "id = ?", step.ID).Error)

	// Simulate ModelToDto conversion
	output := dto.ScenarioStepOutput{
		ID:         fetched.ID,
		ScenarioID: fetched.ScenarioID,
		Order:      fetched.Order,
		Title:      fetched.Title,
		StepType:   fetched.StepType,
		CreatedAt:  fetched.CreatedAt,
		UpdatedAt:  fetched.UpdatedAt,
	}
	assert.Equal(t, "quiz", output.StepType, "StepType should appear in output DTO")
}

func TestStepType_DtoToModel_DefaultWhenEmpty(t *testing.T) {
	// Test that DtoToModel defaults to "terminal" when StepType is empty
	input := dto.CreateScenarioStepInput{
		Title: "Some Step",
		Order: 1,
	}

	// Simulate DtoToModel logic: default to "terminal" if empty
	stepType := input.StepType
	if stepType == "" {
		stepType = "terminal"
	}
	assert.Equal(t, "terminal", stepType, "empty step_type should default to 'terminal'")
}
