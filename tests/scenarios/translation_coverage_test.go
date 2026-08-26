package scenarios_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// coverageScenario builds a three-step scenario declaring one extra locale.
func coverageScenario(t *testing.T, locales string) (*gorm.DB, models.Scenario, []models.ScenarioStep) {
	t.Helper()
	db := freshTestDB(t)

	scenario := models.Scenario{
		Name:          "coverage",
		Title:         "Coverage",
		InstanceType:  "debian",
		CreatedByID:   "creator-1",
		DefaultLocale: "en",
		Locales:       locales,
	}
	require.NoError(t, db.Create(&scenario).Error)

	steps := make([]models.ScenarioStep, 3)
	for i := range steps {
		steps[i] = models.ScenarioStep{
			ScenarioID:  scenario.ID,
			Order:       i,
			Title:       "Step",
			TextContent: "Go to the Cellar.",
			HintContent: "Use cd.",
		}
		require.NoError(t, db.Create(&steps[i]).Error)
	}
	return db, scenario, steps
}

func translateStep(t *testing.T, db *gorm.DB, step models.ScenarioStep, hash string) {
	t.Helper()
	// Every field the source fills: these tests mean a step that is finished,
	// and leaving the hint out would now make it partial rather than translated.
	require.NoError(t, db.Create(&models.ScenarioStepTranslation{
		StepID:      step.ID,
		Locale:      "fr",
		Title:       "Etape",
		TextContent: "Allez a la Cave.",
		HintContent: "Utilisez cd.",
		SourceHash:  hash,
	}).Error)
}

func coverageFor(t *testing.T, db *gorm.DB, scenarioID uuid.UUID, locale string) services.LocaleCoverage {
	t.Helper()
	all, err := services.TranslationCoverage(db, scenarioID)
	require.NoError(t, err)
	for _, c := range all {
		if c.Locale == locale {
			return c
		}
	}
	t.Fatalf("no coverage reported for %q", locale)
	return services.LocaleCoverage{}
}

// A locale nothing has been written for is entirely missing, and not complete.
func TestTranslationCoverage_NothingTranslated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, _ := coverageScenario(t, `["fr"]`)

	coverage := coverageFor(t, db, scenario.ID, "fr")

	assert.Equal(t, 3, coverage.TotalSteps)
	assert.Equal(t, 0, coverage.Translated)
	assert.Equal(t, 3, coverage.Missing)
	assert.False(t, coverage.Complete)
}

// Every step translated against its current source is complete.
func TestTranslationCoverage_FullyTranslated_IsComplete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["fr"]`)
	for _, step := range steps {
		translateStep(t, db, step, services.StepSourceHash(step))
	}

	coverage := coverageFor(t, db, scenario.ID, "fr")

	assert.Equal(t, 3, coverage.Translated)
	assert.Equal(t, 0, coverage.Missing)
	assert.Equal(t, 0, coverage.Stale)
	assert.True(t, coverage.Complete)
}

// Editing the source after translating must show up as stale. This is the
// whole reason SourceHash exists: without it the French reads fine and quietly
// describes something that has changed.
func TestTranslationCoverage_SourceEditedAfterTranslating_IsStale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["fr"]`)
	for _, step := range steps {
		translateStep(t, db, step, services.StepSourceHash(step))
	}
	require.NoError(t, db.Model(&steps[0]).Update("text_content", "Go to the Wine Cellar instead.").Error)

	coverage := coverageFor(t, db, scenario.ID, "fr")

	assert.Equal(t, 3, coverage.Translated)
	assert.Equal(t, 1, coverage.Stale)
	assert.False(t, coverage.Complete, "a stale step means the locale is not safe to launch")
}

// A translation recorded without a source reference cannot be shown as current.
// Reporting it as stale is the safe direction: the alternative is telling a
// trainer a translation is up to date when nothing checked whether it is.
func TestTranslationCoverage_TranslationWithoutSourceHash_CountsAsStale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["fr"]`)
	translateStep(t, db, steps[0], "")

	coverage := coverageFor(t, db, scenario.ID, "fr")

	assert.Equal(t, 1, coverage.Translated)
	assert.Equal(t, 1, coverage.Stale)
}

// The default locale is the content itself, so it is always complete and is
// never reported as needing translation.
func TestTranslationCoverage_DefaultLocale_IsAlwaysComplete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, _ := coverageScenario(t, `["en","fr"]`)

	coverage := coverageFor(t, db, scenario.ID, "en")

	assert.Equal(t, 3, coverage.Translated)
	assert.Equal(t, 0, coverage.Missing)
	assert.True(t, coverage.Complete)
}

// A scenario declaring no locales reports none: nothing to translate, and no
// invented rows for languages nobody asked for.
func TestTranslationCoverage_NoLocalesDeclared_ReportsNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, _ := coverageScenario(t, "")

	all, err := services.TranslationCoverage(db, scenario.ID)

	require.NoError(t, err)
	assert.Empty(t, all)
}

// The editor marks each step in its list, so the coverage report says what
// state every step is in — not just how many are in each.
//
// Per step and server-side on purpose: deciding staleness needs the source
// hash, and a client that recomputed it would be a second implementation of the
// one rule that says whether a translation still matches its source.
func TestTranslationCoverage_ReportsStatePerStep(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en","fr"]`)
	translateStep(t, db, steps[0], services.StepSourceHash(steps[0])) // current
	translateStep(t, db, steps[1], "written-against-something-else")  // stale
	// steps[2] left untranslated

	coverage := coverageFor(t, db, scenario.ID, "fr")

	require.Len(t, coverage.Steps, 3)
	byStep := map[uuid.UUID]string{}
	for _, s := range coverage.Steps {
		byStep[s.StepID] = s.State
	}
	assert.Equal(t, "translated", byStep[steps[0].ID])
	assert.Equal(t, "stale", byStep[steps[1].ID])
	assert.Equal(t, "missing", byStep[steps[2].ID])
}

// The default locale needs no per-step detail: it is the content itself, and
// every step is trivially current.
func TestTranslationCoverage_DefaultLocale_ReportsEveryStepTranslated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, _ := coverageScenario(t, `["en","fr"]`)

	coverage := coverageFor(t, db, scenario.ID, "en")

	require.Len(t, coverage.Steps, 3)
	for _, s := range coverage.Steps {
		assert.Equal(t, "translated", s.State)
	}
}

// A translation that fills only some of what the source fills is not a
// translated step.
//
// Counting it as one is how a language comes to be offered while most of its
// steps still read in the original: a title in French over an English body is
// worse than an honest gap, because nothing reports it and the learner meets it
// mid-run.
func TestTranslationCoverage_TitleOnlyTranslation_IsNotComplete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en","fr"]`)
	for _, step := range steps {
		require.NoError(t, db.Create(&models.ScenarioStepTranslation{
			StepID: step.ID, Locale: "fr",
			Title:      "Titre",
			SourceHash: services.StepSourceHash(step),
		}).Error)
	}

	coverage := coverageFor(t, db, scenario.ID, "fr")

	assert.False(t, coverage.Complete, "a French title over an English body is not a French step")
	assert.Equal(t, 3, coverage.Partial)
	for _, s := range coverage.Steps {
		assert.Equal(t, "partial", s.State)
	}
}

// Filling everything the source fills is what counts.
func TestTranslationCoverage_EveryFilledFieldTranslated_IsComplete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en","fr"]`)
	for _, step := range steps {
		require.NoError(t, db.Create(&models.ScenarioStepTranslation{
			StepID: step.ID, Locale: "fr",
			Title: "Titre", TextContent: "Texte", HintContent: "Indice",
			SourceHash: services.StepSourceHash(step),
		}).Error)
	}

	coverage := coverageFor(t, db, scenario.ID, "fr")

	assert.True(t, coverage.Complete)
	assert.Equal(t, 0, coverage.Partial)
}

// A field the source leaves empty is not something to translate, so a step with
// no hint is not held back for want of a translated hint.
func TestTranslationCoverage_SourceWithNoHint_NeedsNoTranslatedHint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en","fr"]`)
	for _, step := range steps {
		require.NoError(t, db.Model(&step).Update("hint_content", "").Error)
		var fresh models.ScenarioStep
		require.NoError(t, db.First(&fresh, "id = ?", step.ID).Error)
		require.NoError(t, db.Create(&models.ScenarioStepTranslation{
			StepID: fresh.ID, Locale: "fr",
			Title: "Titre", TextContent: "Texte",
			SourceHash: services.StepSourceHash(fresh),
		}).Error)
	}

	coverage := coverageFor(t, db, scenario.ID, "fr")

	assert.True(t, coverage.Complete)
}
