// tests/scenarios/stepTypeDerivation_test.go
//
// Bug under test: ScenarioStep.StepType carries gorm:"default:'terminal'", so
// a step imported or seeded without an explicit type is stored as the literal
// string "terminal" — never empty. The frontend's resolvedStepType honours an
// explicit step_type and only falls back to has_flag when the field is EMPTY,
// so the column default made that fallback unreachable: a flag step rendered
// the terminal Verify UI with nowhere to enter the flag, and the scenario
// could not be completed through the interface.
//
// The fix belongs at import/seed time, where the authored shape is known and
// the answer can be stored once, rather than asking every consumer to
// re-derive it from has_flag.
//
// The distinction that makes this delicate: has_flag:true together with an
// explicit step_type:"terminal" is a LEGITIMATE authored combination — a
// terminal step that also drops a flag file via FlagPath. Only the absence of
// a declared type means "infer it".
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

func TestResolveStepType_InfersFlagOnlyWhenNoTypeIsDeclared(t *testing.T) {
	cases := []struct {
		name     string
		declared string
		hasFlag  bool
		want     string
		why      string
	}{
		{
			name:    "flag step with no declared type",
			hasFlag: true,
			want:    "flag",
			why:     "the whole bug: this used to store 'terminal' and hide the flag input",
		},
		{
			name:    "ordinary step with no declared type",
			hasFlag: false,
			want:    "terminal",
			why:     "unchanged default for steps that carry no flag",
		},
		{
			name:     "terminal step that also drops a flag file",
			declared: "terminal",
			hasFlag:  true,
			want:     "terminal",
			why: "a legitimate authored combination (FlagPath), NOT a conflict to " +
				"repair — an explicit type always wins",
		},
		{
			name:     "quiz step that also carries a flag",
			declared: "quiz",
			hasFlag:  true,
			want:     "quiz",
			why:      "an explicit type wins regardless of which type it is",
		},
		{
			name:     "explicit flag step",
			declared: "flag",
			hasFlag:  true,
			want:     "flag",
			why:      "already correct, must stay correct",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := services.ResolveStepType(tc.declared, tc.hasFlag)
			assert.Equal(t, tc.want, got,
				"ResolveStepType(%q, %v) — %s", tc.declared, tc.hasFlag, tc.why)
		})
	}
}

// writeFlagScenarioDir lays out the smallest KillerCoda archive that exercises
// the import path: one step with a per-step has_flag override and no
// extensions.json, which is exactly the shape the affected scenarios use.
func writeFlagScenarioDir(t *testing.T, hasFlag bool) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "step1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "intro.md"), []byte("intro"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "finish.md"), []byte("finish"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step1", "text.md"), []byte("do the thing"), 0o644))

	flagLiteral := "false"
	if hasFlag {
		flagLiteral = "true"
	}
	index := `{
		"title": "Flag Step Import",
		"description": "step_type derivation",
		"difficulty": "beginner",
		"time": "5m",
		"details": {
			"intro": {"text": "intro.md"},
			"steps": [
				{"title": "Find the flag", "text": "step1/text.md", "has_flag": ` + flagLiteral + `}
			],
			"finish": {"text": "finish.md"}
		},
		"backend": {"imageid": "debian:12"}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.json"), []byte(index), 0o644))
	return dir
}

func TestImportScenario_StoresFlagStepsAsFlagType(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	scenario, err := importer.ImportFromDirectory(
		writeFlagScenarioDir(t, true), "importer-flag-user", nil, "")
	require.NoError(t, err)

	var step models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", scenario.ID).First(&step).Error)
	assert.True(t, step.HasFlag, "the imported step should carry its flag")
	assert.Equal(t, "flag", step.StepType,
		"an imported step with a flag and no declared type must be stored as a "+
			"flag step — storing 'terminal' is what left the learner with no "+
			"flag input and an uncompletable scenario")
}

func TestImportScenario_LeavesOrdinaryStepsAsTerminal(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	scenario, err := importer.ImportFromDirectory(
		writeFlagScenarioDir(t, false), "importer-plain-user", nil, "")
	require.NoError(t, err)

	var step models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", scenario.ID).First(&step).Error)
	assert.Equal(t, "terminal", step.StepType,
		"a step with no flag and no declared type is still a terminal step")
}

func TestSeedScenario_StoresFlagStepsAsFlagType(t *testing.T) {
	db := setupTestDB(t)
	seeder := services.NewScenarioSeedService(db)

	scenario, _, err := seeder.SeedScenario(dto.SeedScenarioInput{
		Title:       "Seed Flag Derivation",
		Description: "step_type derivation on the seed path",
		OsType:      "deb",
		Steps: []dto.SeedStepInput{
			{Title: "Flag step, no declared type", HasFlag: true},
			{Title: "Terminal step that drops a flag file", StepType: "terminal", HasFlag: true},
			{Title: "Ordinary step", HasFlag: false},
		},
	}, "seed-flag-user", nil)
	require.NoError(t, err)

	var steps []models.ScenarioStep
	require.NoError(t, db.Where("scenario_id = ?", scenario.ID).
		Order(`"order" ASC`).Find(&steps).Error)
	require.Len(t, steps, 3)

	assert.Equal(t, "flag", steps[0].StepType,
		"a seeded step with a flag and no declared type must be stored as a flag step")
	assert.Equal(t, "terminal", steps[1].StepType,
		"an explicit terminal type wins even with a flag — that combination is a "+
			"terminal step that also drops a flag file, not a mistake")
	assert.Equal(t, "terminal", steps[2].StepType,
		"a step with no flag is unchanged")
}
