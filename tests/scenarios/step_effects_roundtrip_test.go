package scenarios_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// The banner fields are authoring data, so they have to survive every path a
// scenario travels. A field that resets on re-seed or on duplicate turns a
// working scenario into a silent one.

func effectStepInput() dto.SeedStepInput {
	return dto.SeedStepInput{
		Title:       "Level with banners",
		IntroEffect: "beams",
		IntroText:   "Niveau 2 débloqué",
		OutroEffect: "decrypt",
		OutroText:   "Niveau 2 terminé",
	}
}

func TestSeedScenario_PersistsStepBannerFields(t *testing.T) {
	db := setupTestDB(t)

	scenario, _, err := services.NewScenarioSeedService(db).SeedScenario(dto.SeedScenarioInput{
		Title:        "Seeded banners",
		InstanceType: "ubuntu:22.04",
		Steps:        []dto.SeedStepInput{{Title: "Plain step"}, effectStepInput()},
	}, "creator-1", nil)
	require.NoError(t, err)

	var steps []models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", scenario.ID).Order("\"order\" ASC").Find(&steps).Error)
	require.Len(t, steps, 2)

	assert.Empty(t, steps[0].IntroEffect, "a step that configures nothing stays empty")
	assert.Empty(t, steps[0].OutroText)
	assert.Equal(t, "beams", steps[1].IntroEffect)
	assert.Equal(t, "Niveau 2 débloqué", steps[1].IntroText)
	assert.Equal(t, "decrypt", steps[1].OutroEffect)
	assert.Equal(t, "Niveau 2 terminé", steps[1].OutroText)
}

func TestExportAsJSON_CarriesStepBannerFieldsAndRebindsAsSeedInput(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "export-banners",
		Title:        "Export banners",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		Steps: []models.ScenarioStep{{
			Order:       0,
			Title:       "Level with banners",
			IntroEffect: "beams",
			IntroText:   "Niveau 1",
			OutroEffect: "decrypt",
			OutroText:   "Niveau 1 terminé",
		}},
	}
	require.NoError(t, db.Create(&scenario).Error)

	out, err := services.NewScenarioExportService(db).ExportAsJSON(scenario.ID)
	require.NoError(t, err)
	require.Len(t, out.Steps, 1)
	assert.Equal(t, "beams", out.Steps[0].IntroEffect)
	assert.Equal(t, "Niveau 1 terminé", out.Steps[0].OutroText)

	// The export DTO doubles as the seed input, so the round trip has to bind.
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	var reseed dto.SeedScenarioInput
	require.NoError(t, json.Unmarshal(raw, &reseed))
	require.Len(t, reseed.Steps, 1)
	assert.Equal(t, "beams", reseed.Steps[0].IntroEffect)
	assert.Equal(t, "Niveau 1", reseed.Steps[0].IntroText)
	assert.Equal(t, "decrypt", reseed.Steps[0].OutroEffect)
	assert.Equal(t, "Niveau 1 terminé", reseed.Steps[0].OutroText)
}

func TestScenarioImporter_ReadsStepBannerFieldsFromIndexJSON(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	index, err := importer.ParseIndexJSON([]byte(`{
		"title": "Imported banners",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [
				{"title": "Plain", "text": "step1/text.md"},
				{
					"title": "With banners",
					"text": "step2/text.md",
					"intro_effect": "beams",
					"intro_text": "Niveau 2 débloqué",
					"outro_effect": "decrypt",
					"outro_text": "Niveau 2 terminé"
				}
			],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "ubuntu:22.04"}
	}`))
	require.NoError(t, err)

	scenario, err := importer.BuildScenarioFromIndex(index, t.TempDir(), "creator-1", nil, "upload")
	require.NoError(t, err)
	require.Len(t, scenario.Steps, 2)

	assert.Empty(t, scenario.Steps[0].IntroEffect)
	assert.Equal(t, "beams", scenario.Steps[1].IntroEffect)
	assert.Equal(t, "Niveau 2 débloqué", scenario.Steps[1].IntroText)
	assert.Equal(t, "decrypt", scenario.Steps[1].OutroEffect)
	assert.Equal(t, "Niveau 2 terminé", scenario.Steps[1].OutroText)
}

func TestDuplicateScenario_CopiesStepBannerFields(t *testing.T) {
	db := setupTestDB(t)

	source := models.Scenario{
		Name:         "duplicate-banners",
		Title:        "Duplicate banners",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		Steps: []models.ScenarioStep{{
			Order:       0,
			Title:       "Level with banners",
			IntroEffect: "beams",
			IntroText:   "Niveau 1",
			OutroEffect: "decrypt",
			OutroText:   "Niveau 1 terminé",
		}},
	}
	require.NoError(t, db.Create(&source).Error)

	copied, err := services.NewScenarioDuplicateService(db).DuplicateScenario(source.ID, "creator-2", nil)
	require.NoError(t, err)

	var steps []models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", copied.ID).Order("\"order\" ASC").Find(&steps).Error)
	require.Len(t, steps, 1)
	assert.Equal(t, "beams", steps[0].IntroEffect)
	assert.Equal(t, "Niveau 1", steps[0].IntroText)
	assert.Equal(t, "decrypt", steps[0].OutroEffect)
	assert.Equal(t, "Niveau 1 terminé", steps[0].OutroText)
}
