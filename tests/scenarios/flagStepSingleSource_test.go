// tests/scenarios/flagStepSingleSource_test.go
//
// A step carries a flag exactly when it is a flag step. Two fields used to say
// it independently and drifted:
//
//   - the editor creates step_type "flag" with has_flag false: no flag was
//     generated and the learner was told "no flag found";
//   - step_type "terminal" with has_flag true showed the Verify button, which
//     the backend then refused because the step "requires flag submission".
//
// Both shapes were dead ends. These tests pin the stored row, the generated
// flag and the startup repair of rows written before the rule existed.
package scenarios_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

func createScenarioForFlagRule(t *testing.T, db *gorm.DB, name string) *models.Scenario {
	t.Helper()
	scenario := &models.Scenario{
		Name: name, Title: name, InstanceType: "ubuntu:22.04", CreatedByID: "flag-rule-author",
	}
	require.NoError(t, db.Create(scenario).Error)
	return scenario
}

func TestFlagRule_StoredStep_HasFlagFollowsStepType(t *testing.T) {
	cases := []struct {
		name         string
		stepType     string
		hasFlag      bool
		wantType     string
		wantHasFlag  bool
	}{
		{"flag step created by the editor without has_flag", "flag", false, "flag", true},
		{"terminal step with has_flag is a flag step", "terminal", true, "flag", true},
		{"undeclared type with has_flag is a flag step", "", true, "flag", true},
		{"quiz step never carries a flag", "quiz", true, "quiz", false},
		{"info step never carries a flag", "info", true, "info", false},
		{"ordinary terminal step", "terminal", false, "terminal", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			scenario := createScenarioForFlagRule(t, db, "flag-rule-create")
			step := models.ScenarioStep{
				ScenarioID: scenario.ID, Order: 0, Title: "s", StepType: tc.stepType, HasFlag: tc.hasFlag,
			}
			require.NoError(t, db.Create(&step).Error)

			stored := reloadStep(t, db, step.ID)
			assert.Equal(t, tc.wantType, stored.StepType)
			assert.Equal(t, tc.wantHasFlag, stored.HasFlag)
		})
	}
}

func TestFlagRule_PatchStepType_RewritesHasFlag(t *testing.T) {
	cases := []struct {
		name        string
		start       models.ScenarioStep
		patch       map[string]any
		wantType    string
		wantHasFlag bool
	}{
		{
			name:     "flag to terminal drops the flag",
			start:    models.ScenarioStep{StepType: "flag", HasFlag: true},
			patch:    map[string]any{"step_type": "terminal"},
			wantType: "terminal", wantHasFlag: false,
		},
		{
			name:     "terminal to flag gains the flag",
			start:    models.ScenarioStep{StepType: "terminal"},
			patch:    map[string]any{"step_type": "flag"},
			wantType: "flag", wantHasFlag: true,
		},
		{
			name:     "editor resending has_flag false on a flag step",
			start:    models.ScenarioStep{StepType: "flag", HasFlag: true},
			patch:    map[string]any{"step_type": "flag", "has_flag": false},
			wantType: "flag", wantHasFlag: true,
		},
		{
			name:     "has_flag alone on a flag step cannot remove its flag",
			start:    models.ScenarioStep{StepType: "flag", HasFlag: true},
			patch:    map[string]any{"has_flag": false},
			wantType: "flag", wantHasFlag: true,
		},
		{
			name:     "legacy has_flag checkbox on a terminal step makes it a flag step",
			start:    models.ScenarioStep{StepType: "terminal"},
			patch:    map[string]any{"step_type": "terminal", "has_flag": true},
			wantType: "flag", wantHasFlag: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			scenario := createScenarioForFlagRule(t, db, "flag-rule-patch")
			step := tc.start
			step.ScenarioID, step.Title = scenario.ID, "patched"
			require.NoError(t, db.Create(&step).Error)

			router := setupArchiveRouter(t, db, "platform-admin", []string{"administrator"})
			resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+step.ID.String(), tc.patch)
			require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())

			stored := reloadStep(t, db, step.ID)
			assert.Equal(t, tc.wantType, stored.StepType)
			assert.Equal(t, tc.wantHasFlag, stored.HasFlag)
		})
	}
}

// The scenarios already in production (linux-rogueLite, gameshell-basics step
// 13) are seeded with has_flag and no step_type. An unrelated edit must not
// cost them their flag.
func TestFlagRule_SeededFlagStep_PatchTitleOnly_KeepsItsFlag(t *testing.T) {
	db := freshTestDB(t)
	scenario, _, err := services.NewScenarioSeedService(db).SeedScenario(dto.SeedScenarioInput{
		Title:        "Seeded Flag Scenario",
		OsType:       "deb",
		FlagsEnabled: true,
		Steps: []dto.SeedStepInput{
			{Title: "Find the day", HasFlag: true, BackgroundScript: "echo OCF_ANSWER: tuesday"},
		},
	}, "seed-author", nil)
	require.NoError(t, err)

	var step models.ScenarioStep
	require.NoError(t, db.First(&step, "scenario_id = ?", scenario.ID).Error)
	require.Equal(t, "flag", step.StepType)

	router := setupArchiveRouter(t, db, "platform-admin", []string{"administrator"})
	resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+step.ID.String(),
		map[string]any{"title": "Find the weekday"})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())

	stored := reloadStep(t, db, step.ID)
	assert.Equal(t, "Find the weekday", stored.Title)
	assert.Equal(t, "flag", stored.StepType)
	assert.True(t, stored.HasFlag)

	flags := startRunWithRealFlags(t, db, scenario.ID)
	require.Len(t, flags, 1, "the edited step still gets its flag")
	assert.Equal(t, step.Order, flags[0].StepOrder)
}

// A scenario built in the editor has flags_enabled false; its flag steps must
// still be playable.
func TestFlagRule_FlagsDisabledScenario_FlagStepStillGetsItsFlag(t *testing.T) {
	db := freshTestDB(t)
	scenario := createScenarioForFlagRule(t, db, "editor-built-flags")
	require.False(t, scenario.FlagsEnabled)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "find it", StepType: "flag",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 1, Title: "verify it", StepType: "terminal",
	}).Error)

	flags := startRunWithRealFlags(t, db, scenario.ID)
	require.Len(t, flags, 1, "exactly the flag step gets a flag")
	assert.Equal(t, 0, flags[0].StepOrder)
	assert.Regexp(t, `^FLAG\{[0-9a-f]{16}\}$`, flags[0].ExpectedFlag)
}

func TestFlagRule_StartupRepair_FixesRowsWrittenBeforeTheRule(t *testing.T) {
	db := freshTestDB(t)
	scenario := createScenarioForFlagRule(t, db, "flag-rule-repair")
	raw := db.Session(&gorm.Session{SkipHooks: true})
	seed := map[string]models.ScenarioStep{
		"terminal+flag": {StepType: "terminal", HasFlag: true},
		"flag-noflag":   {StepType: "flag", HasFlag: false},
		"quiz+flag":     {StepType: "quiz", HasFlag: true},
		"terminal":      {StepType: "terminal", HasFlag: false},
	}
	ids := map[string]uuid.UUID{}
	order := 0
	for name, step := range seed {
		// SkipHooks also skips BaseModel's id generation.
		step.ID, step.ScenarioID, step.Title, step.Order = uuid.New(), scenario.ID, name, order
		order++
		require.NoError(t, raw.Create(&step).Error)
		ids[name] = step.ID
	}
	require.True(t, reloadStep(t, db, ids["terminal+flag"]).HasFlag, "fixture must bypass the rule")
	require.False(t, reloadStep(t, db, ids["flag-noflag"]).HasFlag, "fixture must bypass the rule")

	for range 2 { // idempotent
		models.MigrateFlagStepConsistency(db)
	}

	want := map[string]struct {
		stepType string
		hasFlag  bool
	}{
		"terminal+flag": {"flag", true},
		"flag-noflag":   {"flag", true},
		"quiz+flag":     {"quiz", false},
		"terminal":      {"terminal", false},
	}
	for name, w := range want {
		stored := reloadStep(t, db, ids[name])
		assert.Equal(t, w.stepType, stored.StepType, name)
		assert.Equal(t, w.hasFlag, stored.HasFlag, name)
	}
}
