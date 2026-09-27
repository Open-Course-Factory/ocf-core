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

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// writeHostnameScenarioDir writes a minimal KillerCoda scenario whose
// extensions.ocf block is given verbatim (without braces).
func writeHostnameScenarioDir(t *testing.T, ocf string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "step1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "intro.md"), []byte("intro"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "finish.md"), []byte("finish"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step1", "text.md"), []byte("level"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{
		"title": "Hostname Declared In Index",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [{"title": "Level 0", "text": "step1/text.md"}],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "s"},
		"extensions": {"ocf": {`+ocf+`}}
	}`), 0o644))
	return dir
}

// A hostname used to be settable only with PATCH /scenarios/:id after the
// upload, so it lived outside the scenario's repository and was lost whenever
// the scenario was deleted and uploaded again.
func TestImportScenario_SetsHostnameFromIndex(t *testing.T) {
	db := freshTestDB(t)
	importer := services.NewScenarioImporterService(db)

	scenario, err := importer.ImportFromDirectory(writeHostnameScenarioDir(t, `"hostname": "b3car-u1"`), "hostname-user", nil, "")
	require.NoError(t, err)

	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", scenario.ID).Error)
	assert.Equal(t, "b3car-u1", reloaded.Hostname)
}

func TestImportScenario_RejectsInvalidHostname(t *testing.T) {
	db := freshTestDB(t)
	importer := services.NewScenarioImporterService(db)

	for _, bad := range []string{
		`"hostname": "B3CAR"`,               // uppercase
		`"hostname": "-lab"`,                // leading hyphen
		`"hostname": "lab-"`,                // trailing hyphen
		`"hostname": "lab_1"`,               // underscore
		`"hostname": "lab.example"`,         // a label, not an FQDN
		`"hostname": ""`,                    // declared but empty
		`"hostname": "` + longLabel() + `"`, // 64 characters
	} {
		_, err := importer.ImportFromDirectory(writeHostnameScenarioDir(t, bad), "hostname-user", nil, "")
		assert.Error(t, err, "import must be refused for %s", bad)
		if err != nil {
			assert.Contains(t, err.Error(), "extensions.ocf.hostname")
		}
	}
}

func TestImportScenario_ReimportReplacesDeclaredHostname(t *testing.T) {
	db := freshTestDB(t)
	importer := services.NewScenarioImporterService(db)

	first, err := importer.ImportFromDirectory(writeHostnameScenarioDir(t, `"hostname": "old-name"`), "hostname-user", nil, "")
	require.NoError(t, err)
	second, err := importer.ImportFromDirectory(writeHostnameScenarioDir(t, `"hostname": "new-name"`), "hostname-user", nil, "")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "the second import must upsert the same scenario")

	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", second.ID).Error)
	assert.Equal(t, "new-name", reloaded.Hostname)
}

// An older index.json without the field must not wipe a hostname set through
// the API (PATCH /scenarios/:id).
func TestImportScenario_ReimportWithoutHostnameKeepsCurrentOne(t *testing.T) {
	db := freshTestDB(t)
	importer := services.NewScenarioImporterService(db)

	first, err := importer.ImportFromDirectory(writeHostnameScenarioDir(t, `"flags": false`), "hostname-user", nil, "")
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.Scenario{}).Where("id = ?", first.ID).Update("hostname", "set-by-patch").Error)

	second, err := importer.ImportFromDirectory(writeHostnameScenarioDir(t, `"flags": false`), "hostname-user", nil, "")
	require.NoError(t, err)

	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", second.ID).Error)
	assert.Equal(t, "set-by-patch", reloaded.Hostname)
}

// An exported archive must re-import with the same terminal name.
func TestExportArchive_CarriesHostname(t *testing.T) {
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:         "hostname-export",
		Title:        "Hostname Export",
		InstanceType: "s",
		SourceType:   "seed",
		CreatedByID:  "user-1",
		Hostname:     "b3car-u7",
		Steps: []models.ScenarioStep{
			{Order: 0, Title: "Only Step", TextContent: "Content"},
		},
	}
	require.NoError(t, db.Create(&scenario).Error)

	zipBytes, _, err := services.NewScenarioExportService(db).ExportAsArchive(scenario.ID)
	require.NoError(t, err)
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	var index services.KillerCodaIndex
	found := false
	for _, f := range r.File {
		if f.Name != "index.json" {
			continue
		}
		found = true
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
		require.NoError(t, json.Unmarshal(data, &index))
	}
	require.True(t, found, "zip must contain index.json")
	require.NotNil(t, index.Extensions)
	require.NotNil(t, index.Extensions.OCF)
	require.NotNil(t, index.Extensions.OCF.Hostname)
	assert.Equal(t, "b3car-u7", *index.Extensions.OCF.Hostname)
}

func longLabel() string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
