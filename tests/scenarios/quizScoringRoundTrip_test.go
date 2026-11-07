package scenarios_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// partialQuizScenario is a one-step quiz scored partially with no penalty —
// the zero penalty is the value GORM's column default would silently eat.
func partialQuizScenario(t *testing.T, name string) models.Scenario {
	t.Helper()
	scenario := models.Scenario{
		Name: name, Title: name, InstanceType: "ubuntu:22.04", SourceType: "seed", CreatedByID: "user-1",
		Steps: []models.ScenarioStep{{
			Order: 0, Title: "Quiz", StepType: "quiz",
			QuizScoringMode: models.QuizScoringPartial, QuizWrongPenalty: penaltyOf(0),
			Questions: []models.ScenarioStepQuestion{{
				Order: 0, QuestionText: "Q?", QuestionType: "multi_answer",
				Options: `["A","B","C"]`, CorrectAnswer: "[0,2]", Points: 1,
			}},
		}},
	}
	require.NoError(t, freshTestDB(t).Create(&scenario).Error)
	return scenario
}

func loadOnlyStep(t *testing.T, scenarioID any) models.ScenarioStep {
	t.Helper()
	var step models.ScenarioStep
	require.NoError(t, sharedTestDB.Where("scenario_id = ?", scenarioID).First(&step).Error)
	return step
}

func assertPartialNoPenalty(t *testing.T, step models.ScenarioStep) {
	t.Helper()
	assert.Equal(t, models.QuizScoringPartial, step.QuizScoringMode)
	require.NotNil(t, step.QuizWrongPenalty, "a zero penalty must survive, not fall back to the column default")
	assert.InDelta(t, 0.0, *step.QuizWrongPenalty, 1e-9)
}

func TestQuizScoring_StoredZeroPenalty_IsNotReplacedByDefault(t *testing.T) {
	scenario := partialQuizScenario(t, "zero-penalty")
	assertPartialNoPenalty(t, loadOnlyStep(t, scenario.ID))
}

func TestQuizScoring_JSONExportReseed_KeepsScoringSettings(t *testing.T) {
	original := partialQuizScenario(t, "quiz-scoring-json")

	export, err := services.NewScenarioExportService(sharedTestDB).ExportAsJSON(original.ID)
	require.NoError(t, err)
	raw, err := json.Marshal(export)
	require.NoError(t, err)
	var reimport dto.SeedScenarioInput
	require.NoError(t, json.Unmarshal(raw, &reimport))
	reimport.Title = "quiz-scoring-json-copy"

	copied, _, err := services.NewScenarioSeedService(sharedTestDB).SeedScenario(reimport, "user-2", nil)
	require.NoError(t, err)
	assertPartialNoPenalty(t, loadOnlyStep(t, copied.ID))
}

func TestQuizScoring_ArchiveExportImport_KeepsScoringSettings(t *testing.T) {
	original := partialQuizScenario(t, "quiz-scoring-archive")

	zipBytes, _, err := services.NewScenarioExportService(sharedTestDB).ExportAsArchive(original.ID)
	require.NoError(t, err)
	dir := t.TempDir()
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)
	for _, f := range r.File {
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)
		path := filepath.Join(dir, f.Name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, data, 0o644))
	}
	require.NoError(t, sharedTestDB.Delete(&original).Error)

	imported, err := services.NewScenarioImporterService(sharedTestDB).ImportFromDirectory(dir, "user-1", nil, "upload")
	require.NoError(t, err)
	require.Len(t, imported.Steps, 1)
	assertPartialNoPenalty(t, imported.Steps[0])
	assertPartialNoPenalty(t, loadOnlyStep(t, imported.ID))
}
