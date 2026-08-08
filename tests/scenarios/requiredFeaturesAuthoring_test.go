// tests/scenarios/requiredFeaturesAuthoring_test.go
//
// Bug under test: the RogueLite challenge could not finish provisioning. Its
// setup script installs packages, every apt-get failed with "Temporary failure
// resolving 'deb.debian.org'", and the whole launch died on a bare exit 100.
//
// The container was right and the resolver was right. The ocf-base Incus
// profile is NIC-less on purpose — network is an opt-in feature, granted only
// when the composed session asks for it, which happens only when the scenario
// declares it in RequiredFeatures. And a scenario shipped from the challenges
// repository could not declare it: the importer had no notion of
// required_features at all, so the column was always "" no matter what the
// author wrote. Export dropped it too, along with compatible_instance_types,
// which made an exported archive re-import as a strictly weaker scenario.
//
// These tests pin that the authoring pipeline carries the requirement both
// ways.
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

// writeFeatureScenarioDir lays out a minimal KillerCoda directory whose ocf
// extension block is supplied by the caller.
func writeFeatureScenarioDir(t *testing.T, title, extensions string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "step1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "intro.md"), []byte("intro"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "finish.md"), []byte("finish"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step1", "text.md"), []byte("level"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{
		"title": "`+title+`",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [{"title": "Level 0", "text": "step1/text.md"}],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "s"}`+extensions+`
	}`), 0o644))
	return dir
}

func TestImportScenario_CarriesRequiredFeatures(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	dir := writeFeatureScenarioDir(t, "Needs The Network",
		`, "extensions": {"ocf": {"required_features": ["network"]}}`)

	scenario, err := importer.ImportFromDirectory(dir, "rf-import-user", nil, "")
	require.NoError(t, err)

	features, err := scenario.GetRequiredFeatures()
	require.NoError(t, err)
	assert.Equal(t, []string{"network"}, features,
		"a scenario whose setup script installs packages must be able to say so; "+
			"without it the composed session gets the NIC-less base profile and "+
			"provisioning dies on the first apt-get")
}

func TestImportScenario_WithoutRequiredFeaturesAsksForNothing(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	scenario, err := importer.ImportFromDirectory(
		writeFeatureScenarioDir(t, "Needs Nothing", ""), "rf-import-plain", nil, "")
	require.NoError(t, err)

	assert.Empty(t, scenario.RequiredFeatures,
		"the column's empty value must stay empty rather than becoming \"[]\" — "+
			"network is opt-in and silence must not read as a request")
	features, err := scenario.GetRequiredFeatures()
	require.NoError(t, err)
	assert.Empty(t, features)
}

func TestImportScenario_ReimportWithdrawsRequiredFeatures(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	_, err := importer.ImportFromDirectory(
		writeFeatureScenarioDir(t, "Withdrawn Network",
			`, "extensions": {"ocf": {"required_features": ["network"]}}`),
		"rf-withdraw-user", nil, "")
	require.NoError(t, err)

	second, err := importer.ImportFromDirectory(
		writeFeatureScenarioDir(t, "Withdrawn Network", ""), "rf-withdraw-user", nil, "")
	require.NoError(t, err)

	assert.Empty(t, second.RequiredFeatures,
		"required_features is a column, so the upsert's Updates map must list it; "+
			"omitting it silently pins the scenario to whatever it asked for first")
}

// TestExportScenario_KeepsImageAndFeatureRequirements guards the other
// direction. Export is how a scenario moves between environments, so dropping
// these turns a round trip into a downgrade: the archive re-imports with no
// image requirement and no network, resolves onto an arbitrary distribution,
// and fails provisioning exactly the way the RogueLite challenge did.
func TestExportScenario_KeepsImageAndFeatureRequirements(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:             "export-keeps-requirements",
		Title:            "Export Keeps Requirements",
		InstanceType:     "s",
		RequiredFeatures: `["network"]`,
		SourceType:       "builtin",
		CompatibleInstanceTypes: []models.ScenarioInstanceType{
			{InstanceType: "challenge-deb", Priority: 1},
			{InstanceType: "rogueLite", Priority: 0},
		},
		Steps: []models.ScenarioStep{{Order: 0, Title: "Level 0", TextContent: "go"}},
	}
	require.NoError(t, db.Create(&scenario).Error)

	zipBytes, _, err := services.NewScenarioExportService(db).ExportAsArchive(scenario.ID)
	require.NoError(t, err)

	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)
	var indexJSON string
	for _, f := range r.File {
		if f.Name != "index.json" {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
		indexJSON = string(data)
	}
	require.NotEmpty(t, indexJSON, "archive must contain index.json")

	var index services.KillerCodaIndex
	require.NoError(t, json.Unmarshal([]byte(indexJSON), &index))
	require.NotNil(t, index.Extensions, "declared requirements must produce an ocf extension block")
	require.NotNil(t, index.Extensions.OCF)

	assert.Equal(t, []string{"network"}, index.Extensions.OCF.RequiredFeatures)
	assert.Equal(t, []string{"rogueLite", "challenge-deb"}, index.Extensions.OCF.CompatibleInstanceTypes,
		"images must be exported in priority order, not storage order — the first "+
			"entry is the image the author actually wants")
}

// Build-time features: what a scenario needs to be provisioned and not to be
// played. Declaring one must NOT grant it to the learner — that separation is
// the whole point, and it is what lets a scenario install packages while still
// running on a plan with no internet access.

func TestImportScenario_CarriesBuildFeatures(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	dir := writeFeatureScenarioDir(t, "Builds With The Network",
		`, "extensions": {"ocf": {"build_features": ["network"]}}`)

	scenario, err := importer.ImportFromDirectory(dir, "bf-import-user", nil, "")
	require.NoError(t, err)

	build, err := scenario.GetBuildFeatures()
	require.NoError(t, err)
	assert.Equal(t, []string{"network"}, build)

	required, err := scenario.GetRequiredFeatures()
	require.NoError(t, err)
	assert.Empty(t, required,
		"a build-time declaration must not become a session entitlement: the "+
			"container is online while it is built and offline while it is played")
}

func TestImportScenario_BuildAndRequiredFeaturesAreIndependent(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	dir := writeFeatureScenarioDir(t, "Both Kinds",
		`, "extensions": {"ocf": {"required_features": ["persistence"], "build_features": ["network"]}}`)

	scenario, err := importer.ImportFromDirectory(dir, "bf-import-both", nil, "")
	require.NoError(t, err)

	required, err := scenario.GetRequiredFeatures()
	require.NoError(t, err)
	assert.Equal(t, []string{"persistence"}, required)

	build, err := scenario.GetBuildFeatures()
	require.NoError(t, err)
	assert.Equal(t, []string{"network"}, build)
}

func TestScenario_BuildFeaturesMapIsNilWhenNothingIsDeclared(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	scenario, err := importer.ImportFromDirectory(
		writeFeatureScenarioDir(t, "Builds Offline", ""), "bf-import-none", nil, "")
	require.NoError(t, err)

	assert.Empty(t, scenario.BuildFeatures,
		"silence must stay silence rather than becoming \"[]\"")

	m, err := scenario.GetBuildFeaturesMap()
	require.NoError(t, err)
	assert.Nil(t, m,
		"an empty map and a nil map are not the same downstream: the composed "+
			"request omits build_features entirely only when this is nil")
}

func TestScenario_BuildFeaturesMapRejectsAMalformedDeclaration(t *testing.T) {
	scenario := models.Scenario{BuildFeatures: "network"} // not a JSON array

	_, err := scenario.GetBuildFeaturesMap()
	assert.Error(t, err,
		"a bad declaration must be reported, not silently read as no features — "+
			"the launch logs it and provisions without them")
}
