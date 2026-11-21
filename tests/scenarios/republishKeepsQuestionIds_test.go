package scenarios_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// A learner's quiz answers are stored by question id. Republishing a scenario
// used to recreate every question with a new id, so the answers already given in
// a running session stopped matching anything: the answers file, the teacher
// dashboard and the quiz replay lost them (seen on CESI A2, 2026-11-21). A
// question that is still the same question keeps its id.

func decisionScenario(secondType string) dto.SeedScenarioInput {
	second := dto.SeedQuestionInput{Order: 1, QuestionText: "Then?", QuestionType: secondType, CorrectAnswer: "1",
		Options: `["a","b"]`}
	if secondType == "free_text" {
		second.Options, second.CorrectAnswer = "", "ok"
	}
	return dto.SeedScenarioInput{
		Title: "Republish keeps ids", InstanceType: "ubuntu:22.04",
		Steps: []dto.SeedStepInput{{
			Title: "Decide", StepType: "quiz",
			Questions: []dto.SeedQuestionInput{
				{Order: 0, QuestionText: "First?", QuestionType: "multiple_choice", Options: `["a","b"]`, CorrectAnswer: "0"},
				second,
			},
		}},
	}
}

func questionIDs(t *testing.T, db *gorm.DB, scenarioID uuid.UUID) map[int]uuid.UUID {
	t.Helper()
	var scenario models.Scenario
	require.NoError(t, db.Preload("Steps.Questions").First(&scenario, "id = ?", scenarioID).Error)
	require.Len(t, scenario.Steps, 1)
	ids := map[int]uuid.UUID{}
	for _, q := range scenario.Steps[0].Questions {
		ids[q.Order] = q.ID
	}
	return ids
}

func TestRepublish_Seed_KeepsTheIdsOfUnchangedQuestions(t *testing.T) {
	db := freshTestDB(t)
	org := uuid.New()
	seeder := services.NewScenarioSeedService(db)

	first, _, err := seeder.SeedScenario(decisionScenario("multiple_choice"), "author", &org)
	require.NoError(t, err)
	before := questionIDs(t, db, first.ID)

	second, isUpdate, err := seeder.SeedScenario(decisionScenario("free_text"), "author", &org)
	require.NoError(t, err)
	require.True(t, isUpdate)
	after := questionIDs(t, db, second.ID)

	assert.Equal(t, before[0], after[0], "the same question keeps its id")
	assert.NotEqual(t, before[1], after[1], "a question whose type changed is a new question")
}

func TestRepublish_Archive_KeepsTheIdsOfUnchangedQuestions(t *testing.T) {
	db := freshTestDB(t)
	org := uuid.New()
	seeded, _, err := services.NewScenarioSeedService(db).SeedScenario(decisionScenario("multiple_choice"), "author", &org)
	require.NoError(t, err)
	before := questionIDs(t, db, seeded.ID)

	zipBytes, _, err := services.NewScenarioExportService(db).ExportAsArchive(seeded.ID)
	require.NoError(t, err)
	reimported, err := services.NewScenarioImporterService(db).ImportFromDirectory(extractArchiveForTest(t, zipBytes), "author", &org, "upload")
	require.NoError(t, err)
	require.Equal(t, seeded.ID, reimported.ID, "the archive must update the scenario it came from")

	after := questionIDs(t, db, reimported.ID)
	assert.Equal(t, before, after)
}
