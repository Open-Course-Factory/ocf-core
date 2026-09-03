// tests/scenarios/scenarioHealth_test.go
//
// The health report exists because every fault it names was found by a person
// playing a scenario, not by anyone looking at the catalogue. They share a
// shape: the scenario declares something the platform then quietly declines to
// deliver, and nothing errors, logs or looks wrong in the editor.
//
// So the cases here are the real ones:
//
//   - a language declared and not offered, which is what happens after any
//     re-seed that touches English prose: the picker vanishes from the card and
//     the scenario plays in its own language with nobody told;
//   - a scenario with no steps, which launches into an empty container;
//   - a step with no check and no flag, a dead end the learner walks into.
//
// And the case that must stay silent: a healthy scenario produces no findings,
// because a report that lists its own good news is one nobody reads.
package scenarios_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

func findingCodes(health services.ScenarioHealth) []string {
	codes := make([]string, 0, len(health.Findings))
	for _, finding := range health.Findings {
		codes = append(codes, finding.Code)
	}
	return codes
}

func TestScenarioHealth_HealthyScenarioReportsNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en"]`)
	for _, step := range steps {
		require.NoError(t, db.Model(&step).Update("verify_script", "exit 0").Error)
	}

	health, err := services.CheckScenarioHealth(db, scenario)
	require.NoError(t, err)
	assert.Empty(t, health.Findings,
		"a scenario with nothing wrong must produce no findings: %v", findingCodes(health))
}

// The one that cost an afternoon. French was declared, translated, and correct
// in the database — and not offered, because six steps read as stale.
func TestScenarioHealth_DeclaredLocaleThatIsNotOffered(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en","fr"]`)
	for _, step := range steps {
		require.NoError(t, db.Model(&step).Update("verify_script", "exit 0").Error)
	}
	// French is declared and nothing translates it: the launcher will not
	// offer it, and the card will show no picker at all.

	health, err := services.CheckScenarioHealth(db, scenario)
	require.NoError(t, err)

	assert.Contains(t, findingCodes(health), services.HealthLocaleNotOffered,
		"a language the launcher will not offer must be reported, not left for a learner to discover")
	assert.NotContains(t, health.OfferedLocales, "fr")
	for _, finding := range health.Findings {
		if finding.Code == services.HealthLocaleNotOffered {
			assert.Equal(t, "fr", finding.Locale)
			assert.Equal(t, services.HealthBlocking, finding.Severity)
		}
	}
}

func TestScenarioHealth_ScenarioWithNoSteps(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en"]`)
	for _, step := range steps {
		require.NoError(t, db.Delete(&step).Error)
	}

	health, err := services.CheckScenarioHealth(db, scenario)
	require.NoError(t, err)
	assert.Equal(t, []string{services.HealthNoSteps}, findingCodes(health),
		"an empty scenario is one fault, not one fault per question that cannot be asked of it")
}

func TestScenarioHealth_StepWithNoWayToPassIt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en"]`)
	for i, step := range steps {
		if i == 0 {
			continue // left with no verify script and no flag
		}
		require.NoError(t, db.Model(&step).Update("verify_script", "exit 0").Error)
	}

	health, err := services.CheckScenarioHealth(db, scenario)
	require.NoError(t, err)
	assert.Contains(t, findingCodes(health), services.HealthNoVerification)
}

// A step that carries a flag needs no check: the flag is the check.
func TestScenarioHealth_FlagStepIsNotADeadEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en"]`)
	for i, step := range steps {
		if i == 0 {
			require.NoError(t, db.Model(&step).Update("has_flag", true).Error)
			continue
		}
		require.NoError(t, db.Model(&step).Update("verify_script", "exit 0").Error)
	}

	health, err := services.CheckScenarioHealth(db, scenario)
	require.NoError(t, err)
	assert.NotContains(t, findingCodes(health), services.HealthNoVerification)
}

// Found by running the report against production: it called every quiz in the
// catalogue a dead end. A quiz step carries no verify script and no flag
// because answering its questions is how you pass it — five real scenarios,
// all working, all accused. A report that cries wolf on working content is one
// people learn to scroll past, which costs more than not having it at all.
func TestScenarioHealth_QuizStepIsNotADeadEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en"]`)
	for i, step := range steps {
		if i == 0 {
			require.NoError(t, db.Model(&step).Update("step_type", "quiz").Error)
			require.NoError(t, db.Create(&models.ScenarioStepQuestion{
				StepID:       step.ID,
				QuestionText: "What does ls do?",
			}).Error)
			continue
		}
		require.NoError(t, db.Model(&step).Update("verify_script", "exit 0").Error)
	}

	health, err := services.CheckScenarioHealth(db, scenario)
	require.NoError(t, err)
	assert.NotContains(t, findingCodes(health), services.HealthNoVerification,
		"a quiz is passed by answering it, not by a verify script")
}

// A quiz with no questions, though, is a dead end wearing a different hat.
func TestScenarioHealth_QuizWithNoQuestionsIsADeadEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en"]`)
	for i, step := range steps {
		if i == 0 {
			require.NoError(t, db.Model(&step).Update("step_type", "quiz").Error)
			continue
		}
		require.NoError(t, db.Model(&step).Update("verify_script", "exit 0").Error)
	}

	health, err := services.CheckScenarioHealth(db, scenario)
	require.NoError(t, err)
	assert.Contains(t, findingCodes(health), services.HealthNoVerification)
}

// An info step is read, not solved.
func TestScenarioHealth_InfoStepIsNotADeadEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, scenario, steps := coverageScenario(t, `["en"]`)
	for i, step := range steps {
		if i == 0 {
			require.NoError(t, db.Model(&step).Update("step_type", "info").Error)
			continue
		}
		require.NoError(t, db.Model(&step).Update("verify_script", "exit 0").Error)
	}

	health, err := services.CheckScenarioHealth(db, scenario)
	require.NoError(t, err)
	assert.NotContains(t, findingCodes(health), services.HealthNoVerification)
}
