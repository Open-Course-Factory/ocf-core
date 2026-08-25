package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/entityManagement/hooks"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// The source hash is stamped by the server, never accepted from the caller.
//
// It is the only thing that can say a translation has fallen behind its source,
// so a client able to set it is a client able to declare stale work current —
// and the staleness report would then confirm it, which is worse than not
// having one.
func TestTranslationStamp_CreateIgnoresClientHash(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, _, steps := coverageScenario(t, `["en","fr"]`)
	hook := scenarioHooks.NewScenarioStepTranslationStampHook(db)

	translation := &models.ScenarioStepTranslation{
		StepID:     steps[0].ID,
		Locale:     "fr",
		Title:      "Etape",
		SourceHash: "a-hash-the-client-made-up",
	}
	require.NoError(t, hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioStepTranslation",
		HookType:   hooks.BeforeCreate,
		NewEntity:  translation,
	}))

	assert.Equal(t, services.StepSourceHash(steps[0]), translation.SourceHash,
		"the hash must describe the step, not whatever the caller claimed")
}

// Re-saving a translation re-stamps it, which is what marks a step caught up
// again after its source changed.
func TestTranslationStamp_UpdateRestamps(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, _, steps := coverageScenario(t, `["en","fr"]`)
	hook := scenarioHooks.NewScenarioStepTranslationStampHook(db)

	// Snapshot before the edit: GORM's Update writes back through the struct it
	// is given, so steps[0] is itself modified by the line below and can no
	// longer stand in for the old version.
	hashBefore := services.StepSourceHash(steps[0])

	require.NoError(t, db.Model(&steps[0]).Update("text_content", "The cellar has moved.").Error)
	var edited models.ScenarioStep
	require.NoError(t, db.First(&edited, "id = ?", steps[0].ID).Error)

	translation := &models.ScenarioStepTranslation{
		StepID:     steps[0].ID,
		Locale:     "fr",
		Title:      "Etape",
		SourceHash: hashBefore,
	}
	require.NoError(t, hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioStepTranslation",
		HookType:   hooks.BeforeUpdate,
		NewEntity:  translation,
	}))

	assert.Equal(t, services.StepSourceHash(edited), translation.SourceHash)
	assert.NotEqual(t, hashBefore, translation.SourceHash,
		"editing the source must move the hash, or nothing would ever read as stale")
}

// A translation naming a step that does not exist is refused rather than
// stamped with nothing: an empty hash reads as "never checked" everywhere else,
// and would quietly become a permanently stale row.
func TestTranslationStamp_UnknownStep_IsRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, _, _ := coverageScenario(t, `["en","fr"]`)
	hook := scenarioHooks.NewScenarioStepTranslationStampHook(db)

	err := hook.Execute(&hooks.HookContext{
		EntityName: "ScenarioStepTranslation",
		HookType:   hooks.BeforeCreate,
		NewEntity:  &models.ScenarioStepTranslation{Locale: "fr", Title: "Orpheline"},
	})

	require.Error(t, err)
}

// Stamping is what makes a freshly written translation count as current, so the
// coverage report and the editor agree without either recomputing the rule.
func TestTranslationStamp_StampedTranslation_ReadsAsCurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en","fr"]`)
	hook := scenarioHooks.NewScenarioStepTranslationStampHook(db)

	for _, step := range steps {
		translation := &models.ScenarioStepTranslation{StepID: step.ID, Locale: "fr", Title: "Etape"}
		require.NoError(t, hook.Execute(&hooks.HookContext{
			EntityName: "ScenarioStepTranslation",
			HookType:   hooks.BeforeCreate,
			NewEntity:  translation,
		}))
		require.NoError(t, db.Create(translation).Error)
	}

	coverage := coverageFor(t, db, scenario.ID, "fr")

	assert.Equal(t, 0, coverage.Stale)
	assert.True(t, coverage.Complete)
}
