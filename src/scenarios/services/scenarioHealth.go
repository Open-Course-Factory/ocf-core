package services

import (
	"fmt"
	"strings"

	"soli/formations/src/scenarios/models"

	"gorm.io/gorm"
)

// Scenario health: what a scenario claims, measured against what it can do.
//
// Every fault reported here was found the hard way, by a person playing a
// scenario and meeting something the catalogue had not warned them about. They
// share a shape: the scenario declares something — a language, a room, a step —
// and the platform quietly declines to deliver it. Nothing errors, nothing
// logs, and the content looks correct in the editor.
//
// The checks are not new rules. Each one asks a question the launcher or the
// session service already asks at the moment it matters, and asks it early
// enough for someone to fix the answer. Writing a second set of rules here
// would be worse than having no report: it would eventually disagree with the
// one that decides.

// Severities. Blocking means a learner cannot do something the catalogue
// offers; warning means they can, but not as intended.
const (
	HealthBlocking = "blocking"
	HealthWarning  = "warning"
)

// Finding codes. Stable strings, because the front end translates them and a
// reworded sentence must not silently become an untranslated one.
const (
	HealthLocaleNotOffered  = "locale_not_offered"
	HealthLexiconIncomplete = "lexicon_incomplete"
	HealthNoSteps           = "no_steps"
	HealthNoVerification    = "step_without_verification"
)

// ScenarioHealthFinding is one thing wrong with one scenario.
type ScenarioHealthFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	// Locale is set when the finding belongs to one language rather than to the
	// scenario as a whole.
	Locale string `json:"locale,omitempty"`
	// Detail carries the numbers a reader needs to act — which steps, how many
	// — in the platform's language rather than the reader's. The front end
	// writes the sentence from Code; this fills in what it cannot know.
	Detail string `json:"detail,omitempty"`
}

// ScenarioHealth is the report for one scenario.
type ScenarioHealth struct {
	ScenarioID      string                  `json:"scenario_id"`
	Name            string                  `json:"name"`
	Title           string                  `json:"title"`
	IsPublic        bool                    `json:"is_public"`
	DeclaredLocales []string                `json:"declared_locales,omitempty"`
	OfferedLocales  []string                `json:"offered_locales,omitempty"`
	Findings        []ScenarioHealthFinding `json:"findings"`
}

// CheckScenarioHealth reports everything wrong with one scenario.
//
// A scenario with nothing wrong returns no findings rather than a "healthy"
// entry: the report is a list of things to fix, and a page that has to filter
// out its own good news reads as noise.
func CheckScenarioHealth(db *gorm.DB, scenario models.Scenario) (ScenarioHealth, error) {
	report := ScenarioHealth{
		ScenarioID: scenario.ID.String(),
		Name:       scenario.Name,
		Title:      scenario.Title,
		IsPublic:   scenario.IsPublic,
		Findings:   []ScenarioHealthFinding{},
	}

	declared, err := scenario.GetLocales()
	if err != nil {
		return report, fmt.Errorf("cannot read the declared languages: %w", err)
	}
	report.DeclaredLocales = declared

	var steps []models.ScenarioStep
	if err := db.Where("scenario_id = ?", scenario.ID).Order("\"order\" ASC").Find(&steps).Error; err != nil {
		return report, fmt.Errorf("cannot read the steps: %w", err)
	}

	if len(steps) == 0 {
		report.Findings = append(report.Findings, ScenarioHealthFinding{
			Code:     HealthNoSteps,
			Severity: HealthBlocking,
		})
		// Everything below asks a question about steps. Asking it of none would
		// report a second fault that is really the first one restated.
		return report, nil
	}

	// A step with no way to pass it is a dead end the learner reaches with no
	// warning. What counts as "a way" depends on what kind of step it is, and
	// asking only about verify scripts and flags called every quiz in the
	// catalogue a dead end — six questions each, all answerable. A report that
	// cries wolf on working content is one people learn to scroll past, which
	// costs more than not having it.
	stranded := []string{}
	for _, step := range steps {
		if stepHasAWayThrough(db, step) {
			continue
		}
		stranded = append(stranded, fmt.Sprintf("%d", step.Order))
	}
	if len(stranded) > 0 {
		report.Findings = append(report.Findings, ScenarioHealthFinding{
			Code:     HealthNoVerification,
			Severity: HealthBlocking,
			Detail:   strings.Join(stranded, ", "),
		})
	}

	if len(declared) == 0 {
		return report, nil
	}

	// The two questions the launcher itself asks, in the order it asks them.
	offered, err := LaunchableLocales(db, scenario.ID)
	if err != nil {
		return report, fmt.Errorf("cannot resolve the offered languages: %w", err)
	}
	report.OfferedLocales = offered

	coverage, err := TranslationCoverage(db, scenario.ID)
	if err != nil {
		return report, fmt.Errorf("cannot measure translation coverage: %w", err)
	}
	byLocale := map[string]LocaleCoverage{}
	for _, locale := range coverage {
		byLocale[locale.Locale] = locale
	}

	lexicon, err := ValidateLexicon(db, scenario.ID)
	if err != nil {
		return report, fmt.Errorf("cannot validate the vocabulary: %w", err)
	}
	lexiconTrouble := map[string][]string{}
	for _, problem := range lexicon {
		if locale, rest, found := strings.Cut(problem, ":"); found {
			lexiconTrouble[locale] = append(lexiconTrouble[locale], strings.TrimSpace(rest))
		}
	}

	for _, locale := range declared {
		if locale == "" || locale == scenario.DefaultLocale {
			continue
		}
		if problems, bad := lexiconTrouble[locale]; bad {
			report.Findings = append(report.Findings, ScenarioHealthFinding{
				Code:     HealthLexiconIncomplete,
				Severity: HealthBlocking,
				Locale:   locale,
				Detail:   strings.Join(problems, "; "),
			})
		}
		if contains(offered, locale) {
			continue
		}
		// Declared and not offered: the card shows no language picker and the
		// scenario silently plays in its own language. Say which of the three
		// reasons it is, because they need different work.
		report.Findings = append(report.Findings, ScenarioHealthFinding{
			Code:     HealthLocaleNotOffered,
			Severity: HealthBlocking,
			Locale:   locale,
			Detail:   coverageDetail(byLocale[locale]),
		})
	}

	return report, nil
}

// stepHasAWayThrough answers the only question that matters about a step: can
// the learner get past it?
//
// Each kind of step answers differently, and the answer belongs with the kinds
// rather than with the health check — a second opinion about what makes a quiz
// passable is exactly the drift this report exists to catch.
func stepHasAWayThrough(db *gorm.DB, step models.ScenarioStep) bool {
	switch normalizeStepType(step.StepType) {
	case "quiz":
		// Answering is the way through, so a quiz needs questions and nothing
		// else. One with none is a dead end wearing a different hat.
		var questions int64
		if err := db.Model(&models.ScenarioStepQuestion{}).
			Where("step_id = ?", step.ID).Count(&questions).Error; err != nil {
			// Unreadable is not the same as absent: say nothing rather than
			// accuse a step on the strength of a failed query.
			return true
		}
		return questions > 0
	case "info":
		// Nothing to do but read it.
		return true
	}
	return step.VerifyScript != "" || step.VerifyScriptID != nil || step.HasFlag
}

// coverageDetail says, in numbers, why a language is not offered.
func coverageDetail(c LocaleCoverage) string {
	parts := []string{}
	if c.Missing > 0 {
		parts = append(parts, fmt.Sprintf("%d untranslated", c.Missing))
	}
	if c.Partial > 0 {
		parts = append(parts, fmt.Sprintf("%d partly translated", c.Partial))
	}
	if c.Stale > 0 {
		parts = append(parts, fmt.Sprintf("%d behind the source", c.Stale))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

func contains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
