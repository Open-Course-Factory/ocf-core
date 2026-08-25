package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/services"
)

// A scenario that declares no locales offers no choice: it is single-language,
// and a launcher must show no picker rather than one with a single entry.
func TestLaunchableLocales_NoLocalesDeclared_OffersNoChoice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, _ := coverageScenario(t, "")

	locales, err := services.LaunchableLocales(db, scenario.ID)

	require.NoError(t, err)
	assert.Empty(t, locales)
}

// An incomplete locale is not offered. This is the rule the whole coverage
// report exists to serve: a learner sent into a French world who then hits an
// English step is reading about rooms that are not on their disk.
func TestLaunchableLocales_IncompleteLocale_IsNotOffered(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en","fr"]`)
	translateStep(t, db, steps[0], services.StepSourceHash(steps[0]))

	locales, err := services.LaunchableLocales(db, scenario.ID)

	require.NoError(t, err)
	assert.Equal(t, []string{"en"}, locales,
		"one translated step out of three is not a French scenario")
}

// A fully translated locale is offered alongside the default.
func TestLaunchableLocales_CompleteLocale_IsOffered(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en","fr"]`)
	for _, step := range steps {
		translateStep(t, db, step, services.StepSourceHash(step))
	}

	locales, err := services.LaunchableLocales(db, scenario.ID)

	require.NoError(t, err)
	assert.Equal(t, []string{"en", "fr"}, locales)
}

// A locale that went stale stops being offered, without anyone having to
// remember to withdraw it.
func TestLaunchableLocales_StaleLocale_StopsBeingOffered(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en","fr"]`)
	for _, step := range steps {
		translateStep(t, db, step, services.StepSourceHash(step))
	}
	require.NoError(t, db.Model(&steps[1]).Update("text_content", "Now go somewhere else.").Error)

	locales, err := services.LaunchableLocales(db, scenario.ID)

	require.NoError(t, err)
	assert.Equal(t, []string{"en"}, locales,
		"editing the source withdraws the translation until it is caught up")
}

// Starting a session in a language the scenario cannot actually deliver must be
// refused, not silently downgraded to the default. A learner who asked for
// French and got English would read correct text about the wrong world.
func TestStartScenario_UnavailableLocale_IsRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, _ := coverageScenario(t, `["en","fr"]`)
	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	_, err := svc.StartScenario("student-1", scenario.ID, "terminal-locale-refused", "fr")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fr")
}

// The empty locale is every session that exists today and must keep working.
func TestStartScenario_NoLocale_IsAlwaysAllowed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, _ := coverageScenario(t, `["en","fr"]`)
	svc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})

	session, err := svc.StartScenario("student-1", scenario.ID, "terminal-locale-default", "")

	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Empty(t, session.Locale)
}
