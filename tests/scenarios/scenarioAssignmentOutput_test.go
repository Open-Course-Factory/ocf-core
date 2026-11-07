package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// The class scenario tab reads assignments from the generic entity list, so
// the publish state and the exam flag must be on that output too.
func TestScenarioAssignmentOutputs_CarryPublishStateAndExamFlag(t *testing.T) {
	db := freshTestDB(t)
	exam := gradedRunFixture(t, db, true, false)
	learning := gradedRunFixture(t, db, false, false)
	_, err := services.NewTeacherDashboardService(db, nil, nil).PublishScenarioResults(exam.group.ID, exam.scenario.ID)
	require.NoError(t, err)

	var assignments []models.ScenarioAssignment
	require.NoError(t, db.Order("created_at").Find(&assignments).Error)
	require.Len(t, assignments, 2)
	outputs := make([]any, 0, len(assignments))
	for i := range assignments {
		outputs = append(outputs, scenarioRegistration.ScenarioAssignmentToOutput(&assignments[i]))
	}

	require.NoError(t, scenarioRegistration.EnrichScenarioAssignmentOutputs(nil, outputs, db))

	byScenario := map[any]dto.ScenarioAssignmentOutput{}
	for _, o := range outputs {
		out := o.(dto.ScenarioAssignmentOutput)
		byScenario[out.ScenarioID] = out
	}
	assert.True(t, byScenario[exam.scenario.ID].HasExamQuiz)
	assert.NotNil(t, byScenario[exam.scenario.ID].ResultsPublishedAt)
	assert.False(t, byScenario[learning.scenario.ID].HasExamQuiz)
	assert.Nil(t, byScenario[learning.scenario.ID].ResultsPublishedAt)
}
