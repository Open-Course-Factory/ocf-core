package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
)

func TestScenarioStepQuestion_DefaultPoints(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name: "question-default-pts", Title: "Default Points", InstanceType: "ubuntu:22.04", CreatedByID: "u1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "Quiz Step", StepType: "quiz",
	}
	require.NoError(t, db.Create(&step).Error)

	question := models.ScenarioStepQuestion{
		StepID:        step.ID,
		Order:         1,
		QuestionText:  "True or false: /dev/null discards data",
		QuestionType:  "true_false",
		CorrectAnswer: "true",
	}
	require.NoError(t, db.Create(&question).Error)

	var fetched models.ScenarioStepQuestion
	require.NoError(t, db.First(&fetched, "id = ?", question.ID).Error)
	assert.Equal(t, 1, fetched.Points, "default points should be 1")
}
