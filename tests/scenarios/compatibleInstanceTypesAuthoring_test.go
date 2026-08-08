// tests/scenarios/compatibleInstanceTypesAuthoring_test.go
//
// Bug under test: `linux-rogue-lite` has been launching on the generic Debian
// image instead of its purpose-built rogueLite one, in production, for a real
// class — so its sabotage content has never run on the base it was written for.
//
// The resolver is not at fault. resolveDistribution matches
// CompatibleInstanceTypes by name (priority ascending) and only falls back to
// coarse OsType matching when that list is EMPTY — behaviour deliberately
// pinned by TestResolveDistribution_CompatibleInstanceTypes_FallbackToOsType.
// The scenario's list was empty, its os_type "deb" matched the generic Debian
// distribution, and that is exactly what the resolver is designed to do.
//
// The list was empty because it could not be filled: CompatibleInstanceTypes
// was reachable only through the ScenarioInstanceType entity CRUD API. Neither
// import nor seed could express it, so no scenario shipped from the challenges
// repository could ever declare which image it needs. These tests pin that the
// authoring pipeline can now carry the requirement.
package scenarios_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

func loadInstanceTypes(t *testing.T, scenarioID any) []models.ScenarioInstanceType {
	t.Helper()
	var types []models.ScenarioInstanceType
	require.NoError(t, sharedTestDB.Where("scenario_id = ?", scenarioID).
		Order("priority ASC").Find(&types).Error)
	return types
}

func TestImportScenario_CarriesCompatibleInstanceTypes(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "step1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "intro.md"), []byte("intro"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "finish.md"), []byte("finish"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step1", "text.md"), []byte("level"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{
		"title": "Needs Its Own Image",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [{"title": "Level 0", "text": "step1/text.md"}],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "debian:12"},
		"extensions": {
			"ocf": {"compatible_instance_types": ["rogueLite", "debian"]}
		}
	}`), 0o644))

	scenario, err := importer.ImportFromDirectory(dir, "cit-import-user", nil, "")
	require.NoError(t, err)

	types := loadInstanceTypes(t, scenario.ID)
	require.Len(t, types, 2,
		"a scenario declaring the images it can run on must keep that declaration "+
			"through import — dropping it is what left linux-rogue-lite matching "+
			"on os_type alone and landing on the generic Debian image")
	assert.Equal(t, "rogueLite", types[0].InstanceType)
	assert.Equal(t, "debian", types[1].InstanceType)
	assert.Less(t, types[0].Priority, types[1].Priority,
		"declaration order is preference order: the resolver tries these by "+
			"priority ascending, so the first one authored must be tried first")
}

func TestImportScenario_WithoutDeclarationLeavesTheListEmpty(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "step1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "intro.md"), []byte("intro"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "finish.md"), []byte("finish"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step1", "text.md"), []byte("level"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{
		"title": "Happy On Anything Debian",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [{"title": "Level 0", "text": "step1/text.md"}],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "debian:12"}
	}`), 0o644))

	scenario, err := importer.ImportFromDirectory(dir, "cit-import-plain", nil, "")
	require.NoError(t, err)

	assert.Empty(t, loadInstanceTypes(t, scenario.ID),
		"a scenario that declares nothing keeps the existing os_type matching — "+
			"this change is additive and must not invent a requirement")
}

func TestSeedScenario_CarriesCompatibleInstanceTypes(t *testing.T) {
	db := setupTestDB(t)
	seeder := services.NewScenarioSeedService(db)

	scenario, _, err := seeder.SeedScenario(dto.SeedScenarioInput{
		Title:                   "Seeded With Image Requirement",
		OsType:                  "deb",
		CompatibleInstanceTypes: []string{"rogueLite"},
		Steps:                   []dto.SeedStepInput{{Title: "Level 0"}},
	}, "cit-seed-user", nil)
	require.NoError(t, err)

	types := loadInstanceTypes(t, scenario.ID)
	require.Len(t, types, 1)
	assert.Equal(t, "rogueLite", types[0].InstanceType)
}

// TestSeedScenario_ReplacesCompatibleInstanceTypesOnReseed matters because
// seeding is an upsert and is how a fix reaches an already-deployed scenario:
// re-seeding must converge on the declaration, not accumulate old ones.
func TestSeedScenario_ReplacesCompatibleInstanceTypesOnReseed(t *testing.T) {
	db := setupTestDB(t)
	seeder := services.NewScenarioSeedService(db)

	input := dto.SeedScenarioInput{
		Title:                   "Reseeded Image Requirement",
		OsType:                  "deb",
		CompatibleInstanceTypes: []string{"debian"},
		Steps:                   []dto.SeedStepInput{{Title: "Level 0"}},
	}
	_, _, err := seeder.SeedScenario(input, "cit-reseed-user", nil)
	require.NoError(t, err)

	input.CompatibleInstanceTypes = []string{"rogueLite"}
	scenario, isUpdate, err := seeder.SeedScenario(input, "cit-reseed-user", nil)
	require.NoError(t, err)
	require.True(t, isUpdate, "the second seed must update the same scenario")

	types := loadInstanceTypes(t, scenario.ID)
	require.Len(t, types, 1,
		"re-seeding must replace the declaration, not append to it — otherwise a "+
			"corrected scenario keeps matching its old image")
	assert.Equal(t, "rogueLite", types[0].InstanceType)
}
