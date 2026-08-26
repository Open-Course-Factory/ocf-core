package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// Re-seeding must keep a step's identity.
//
// Content is authored in files and pushed repeatedly; translations are written
// once, by hand, and may live only in the database. Replacing every step on
// each seed detaches them silently — the work is still stored, attached to a
// row nothing reads, and the scenario simply reads untranslated again.

func seedTwoSteps(t *testing.T, name, secondText string) *models.Scenario {
	t.Helper()
	seeder := services.NewScenarioSeedService(sharedTestDB)
	scenario, _, err := seeder.SeedScenario(dto.SeedScenarioInput{
		Title:        name,
		InstanceType: "debian",
		Steps: []dto.SeedStepInput{
			{Title: "First", TextContent: "Go to the Cellar."},
			{Title: "Second", TextContent: secondText},
		},
	}, "reseed-user", nil)
	require.NoError(t, err)
	return scenario
}

func TestReseed_KeepsTranslationsAttached(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	freshTestDB(t)

	scenario := seedTwoSteps(t, "reseed-keeps", "Open the Chest.")
	var steps []models.ScenarioStep
	require.NoError(t, sharedTestDB.Where("scenario_id = ?", scenario.ID).Order("\"order\" ASC").Find(&steps).Error)
	require.Len(t, steps, 2)

	require.NoError(t, sharedTestDB.Create(&models.ScenarioStepTranslation{
		StepID: steps[1].ID, Locale: "fr",
		Title: "Deuxieme", TextContent: "Ouvrez le Coffre.",
		SourceHash: services.StepSourceHash(steps[1]),
	}).Error)

	seedTwoSteps(t, "reseed-keeps", "Open the Chest.")

	var attached int64
	require.NoError(t, sharedTestDB.Model(&models.ScenarioStepTranslation{}).
		Joins("JOIN scenario_steps s ON s.id = scenario_step_translations.step_id AND s.deleted_at IS NULL").
		Where("scenario_step_translations.locale = ?", "fr").Count(&attached).Error)
	assert.Equal(t, int64(1), attached, "the translation still belongs to a step that exists")
}

// Editing the source must make the translation stale, not lose it. Losing it
// would be the harsher failure dressed as the milder one: nothing to fix,
// because nothing is left to notice.
func TestReseed_EditedStep_LeavesTheTranslationStale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	freshTestDB(t)

	scenario := seedTwoSteps(t, "reseed-stale", "Open the Chest.")
	var steps []models.ScenarioStep
	require.NoError(t, sharedTestDB.Where("scenario_id = ?", scenario.ID).Order("\"order\" ASC").Find(&steps).Error)

	for _, step := range steps {
		require.NoError(t, sharedTestDB.Create(&models.ScenarioStepTranslation{
			StepID: step.ID, Locale: "fr", Title: "Titre", TextContent: "Texte",
			SourceHash: services.StepSourceHash(step),
		}).Error)
	}
	require.NoError(t, sharedTestDB.Model(&models.Scenario{}).Where("id = ?", scenario.ID).
		Updates(map[string]any{"default_locale": "en", "locales": `["en","fr"]`}).Error)

	seedTwoSteps(t, "reseed-stale", "Open the Chest, carefully.")

	coverage, err := services.TranslationCoverage(sharedTestDB, scenario.ID)
	require.NoError(t, err)
	var french services.LocaleCoverage
	for _, c := range coverage {
		if c.Locale == "fr" {
			french = c
		}
	}
	assert.Equal(t, 2, french.Translated, "both translations survived the re-seed")
	assert.Equal(t, 1, french.Stale, "and the edited one says so")
}

// A step the scenario no longer has goes away, translation and all. Keeping it
// would leave a language reporting work for a step nobody can reach.
func TestReseed_RemovedStep_TakesItsTranslationWithIt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	freshTestDB(t)

	scenario := seedTwoSteps(t, "reseed-shrink", "Open the Chest.")
	var steps []models.ScenarioStep
	require.NoError(t, sharedTestDB.Where("scenario_id = ?", scenario.ID).Order("\"order\" ASC").Find(&steps).Error)
	require.NoError(t, sharedTestDB.Create(&models.ScenarioStepTranslation{
		StepID: steps[1].ID, Locale: "fr", Title: "Deuxieme",
		SourceHash: services.StepSourceHash(steps[1]),
	}).Error)

	seeder := services.NewScenarioSeedService(sharedTestDB)
	_, _, err := seeder.SeedScenario(dto.SeedScenarioInput{
		Title:        "reseed-shrink",
		InstanceType: "debian",
		Steps:        []dto.SeedStepInput{{Title: "First", TextContent: "Go to the Cellar."}},
	}, "reseed-user", nil)
	require.NoError(t, err)

	var live int64
	require.NoError(t, sharedTestDB.Model(&models.ScenarioStep{}).
		Where("scenario_id = ?", scenario.ID).Count(&live).Error)
	assert.Equal(t, int64(1), live)

	var orphaned int64
	require.NoError(t, sharedTestDB.Model(&models.ScenarioStepTranslation{}).
		Where("step_id = ?", steps[1].ID).Count(&orphaned).Error)
	assert.Equal(t, int64(0), orphaned, "no translation left pointing at a step that is gone")
}
