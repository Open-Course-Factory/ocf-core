package scenarios_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// The provisioning knobs (background_timeout_seconds, background_async) are
// authoring data: they have to survive every path a scenario travels — seed,
// JSON export, archive export, duplication. A field that silently resets on
// re-seed turns a working challenge into a timing-out one.

func TestSeedScenario_PersistsStepProvisioningFields(t *testing.T) {
	db := setupTestDB(t)

	seedSvc := services.NewScenarioSeedService(db)
	scenario, _, err := seedSvc.SeedScenario(dto.SeedScenarioInput{
		Title:        "Seeded provisioning knobs",
		InstanceType: "ubuntu:22.04",
		Steps: []dto.SeedStepInput{
			{Title: "Quick step", BackgroundScript: "echo quick"},
			{
				Title:                    "Heavy step",
				BackgroundScript:         "echo heavy",
				BackgroundTimeoutSeconds: 120,
				BackgroundAsync:          true,
			},
		},
	}, "creator-1", nil)
	require.NoError(t, err)

	var steps []models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", scenario.ID).Order("\"order\" ASC").Find(&steps).Error)
	require.Len(t, steps, 2)

	assert.Equal(t, 0, steps[0].BackgroundTimeoutSeconds, "an unset timeout stays 0 — the engine default")
	assert.False(t, steps[0].BackgroundAsync)
	assert.Equal(t, 120, steps[1].BackgroundTimeoutSeconds)
	assert.True(t, steps[1].BackgroundAsync)
}

func TestExportAsJSON_CarriesStepProvisioningFields(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "export-provisioning",
		Title:        "Export provisioning",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Step 1", BackgroundScript: "echo one"},
			{
				Order:                    1,
				Title:                    "Step 2",
				BackgroundScript:         "echo two",
				BackgroundTimeoutSeconds: 90,
				BackgroundAsync:          true,
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	out, err := services.NewScenarioExportService(db).ExportAsJSON(scenario.ID)
	require.NoError(t, err)
	require.Len(t, out.Steps, 2)

	assert.Equal(t, 0, out.Steps[0].BackgroundTimeoutSeconds)
	assert.False(t, out.Steps[0].BackgroundAsync)
	assert.Equal(t, 90, out.Steps[1].BackgroundTimeoutSeconds)
	assert.True(t, out.Steps[1].BackgroundAsync)

	// The export DTO doubles as the seed input, so the round trip has to bind.
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	var reseed dto.SeedScenarioInput
	require.NoError(t, json.Unmarshal(raw, &reseed))
	require.Len(t, reseed.Steps, 2)
	assert.Equal(t, 90, reseed.Steps[1].BackgroundTimeoutSeconds)
	assert.True(t, reseed.Steps[1].BackgroundAsync)
}

func TestExportAsArchive_WritesProvisioningFieldsIntoIndexJSON(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "archive-provisioning",
		Title:        "Archive provisioning",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		Steps: []models.ScenarioStep{
			{
				Order:                    0,
				Title:                    "Heavy step",
				TextContent:              "do the thing",
				BackgroundScript:         "echo heavy",
				BackgroundTimeoutSeconds: 120,
				BackgroundAsync:          true,
			},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	archive, _, err := services.NewScenarioExportService(db).ExportAsArchive(scenario.ID)
	require.NoError(t, err)

	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)

	var indexJSON []byte
	for _, f := range reader.File {
		if f.Name != "index.json" {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		indexJSON, err = io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
	}
	require.NotEmpty(t, indexJSON, "archive must contain index.json")

	var index struct {
		Details struct {
			Steps []struct {
				BackgroundTimeoutSeconds int  `json:"background_timeout_seconds"`
				BackgroundAsync          bool `json:"background_async"`
			} `json:"steps"`
		} `json:"details"`
	}
	require.NoError(t, json.Unmarshal(indexJSON, &index))
	require.Len(t, index.Details.Steps, 1)
	assert.Equal(t, 120, index.Details.Steps[0].BackgroundTimeoutSeconds)
	assert.True(t, index.Details.Steps[0].BackgroundAsync)
}

func TestScenarioImporter_ReadsStepProvisioningFieldsFromIndexJSON(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	index, err := importer.ParseIndexJSON([]byte(`{
		"title": "Imported provisioning",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [
				{"title": "Quick step", "text": "step1/text.md"},
				{
					"title": "Heavy step",
					"text": "step2/text.md",
					"background": "step2/background.sh",
					"background_timeout_seconds": 120,
					"background_async": true
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

	assert.Equal(t, 0, scenario.Steps[0].BackgroundTimeoutSeconds)
	assert.False(t, scenario.Steps[0].BackgroundAsync)
	assert.Equal(t, 120, scenario.Steps[1].BackgroundTimeoutSeconds)
	assert.True(t, scenario.Steps[1].BackgroundAsync)
}

func TestDuplicateScenario_CopiesStepProvisioningFields(t *testing.T) {
	db := setupTestDB(t)

	source := models.Scenario{
		Name:         "duplicate-provisioning",
		Title:        "Duplicate provisioning",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
		Steps: []models.ScenarioStep{
			{
				Order:                    0,
				Title:                    "Heavy step",
				BackgroundScript:         "echo heavy",
				BackgroundTimeoutSeconds: 150,
				BackgroundAsync:          true,
			},
		},
	}
	require.NoError(t, db.Create(&source).Error)

	copied, err := services.NewScenarioDuplicateService(db).DuplicateScenario(source.ID, "creator-2", nil)
	require.NoError(t, err)

	var steps []models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", copied.ID).Order("\"order\" ASC").Find(&steps).Error)
	require.Len(t, steps, 1)
	assert.Equal(t, 150, steps[0].BackgroundTimeoutSeconds)
	assert.True(t, steps[0].BackgroundAsync)
}
