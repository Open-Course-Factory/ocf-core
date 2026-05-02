package scenarios_test

// Red-phase tests for ScenarioStepProgress quiz columns (#283).
//
// These tests assert that the underlying scenario_step_progress table gains
// three new columns to support quiz playback:
//   - step_type     varchar — denormalized from ScenarioStep.StepType so the
//                              teacher dashboard can filter without joining
//   - quiz_score    float   — nullable, percentage in [0, 1]
//   - quiz_answers  text    — nullable, JSON map of question_id → submitted answer
//
// Each test introspects the SQLite migration produced from the GORM model
// definition in main_test.go. They will fail until ScenarioStepProgress is
// extended with the matching fields.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// columnExists checks the SQLite info schema for a column on the given table.
func columnExists(t *testing.T, table, column string) bool {
	t.Helper()
	type colInfo struct {
		Name string
	}
	var rows []colInfo
	require.NoError(t, sharedTestDB.Raw(
		"SELECT name FROM pragma_table_info(?)", table,
	).Scan(&rows).Error)
	for _, r := range rows {
		if r.Name == column {
			return true
		}
	}
	return false
}

// TestScenarioStepProgress_HasStepTypeColumn — ScenarioStepProgress must
// expose a step_type column so the dev agent can record which kind of step a
// progress row represents (terminal/flag/info/quiz).
func TestScenarioStepProgress_HasStepTypeColumn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	assert.True(t, columnExists(t, "scenario_step_progress", "step_type"),
		"scenario_step_progress must have a step_type column for step-type-aware progress tracking")
}

// TestScenarioStepProgress_HasQuizScoreColumn — quiz_score column must exist
// so quiz attempts can be scored and surfaced on the teacher dashboard.
func TestScenarioStepProgress_HasQuizScoreColumn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	assert.True(t, columnExists(t, "scenario_step_progress", "quiz_score"),
		"scenario_step_progress must have a quiz_score column to record quiz scoring (nullable float in [0,1])")
}

// TestScenarioStepProgress_HasQuizAnswersColumn — quiz_answers column must
// exist to store the JSON-encoded answers payload submitted by the student.
func TestScenarioStepProgress_HasQuizAnswersColumn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	assert.True(t, columnExists(t, "scenario_step_progress", "quiz_answers"),
		"scenario_step_progress must have a quiz_answers column to store the submitted answers JSON")
}
