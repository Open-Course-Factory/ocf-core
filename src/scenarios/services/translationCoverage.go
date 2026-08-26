package services

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LocaleCoverage says how much of a scenario a language actually covers.
//
// It exists because a locale is either safe to launch or it is not, and that is
// not a per-field question. A learner reading one English step in a French
// world is reading about rooms that do not exist on their disk — the fallback
// that keeps a half-translated scenario readable is exactly what makes it
// unplayable. So the launcher offers complete locales only, and this is what
// tells it which those are.
type LocaleCoverage struct {
	Locale     string `json:"locale"`
	TotalSteps int    `json:"total_steps"`
	Translated int    `json:"translated"`
	Stale      int    `json:"stale"`
	Missing    int    `json:"missing"`

	// Partial counts steps whose translation fills less than the source does —
	// a French title over an English body. Counted apart from missing because
	// they are different jobs: one is untouched, the other is half done and
	// looks finished.
	Partial  int  `json:"partial"`
	Complete bool `json:"complete"`

	// Steps says what state each step is in, in scenario order, so an editor
	// can mark its list without a second request. Decided here rather than in
	// the client: staleness needs the source hash, and a client recomputing it
	// would be a second implementation of the one rule that says whether a
	// translation still matches what it was written from.
	Steps []StepTranslationState `json:"steps"`
}

// StepTranslationState is one step's standing in one locale.
type StepTranslationState struct {
	StepID uuid.UUID `json:"step_id"`
	Order  int       `json:"order"`
	State  string    `json:"state"` // translated, stale, or missing
}

const (
	StateTranslated = "translated"
	StatePartial    = "partial"
	StateStale      = "stale"
	StateMissing    = "missing"
)

// coversTheSource reports whether a translation fills everything the step
// fills.
//
// A field the source leaves empty is not something to translate, so a step with
// no hint is not held back for want of a translated hint. A field the source
// fills and the translation does not is the learner reading the original
// mid-run, which is exactly what a completeness gate is for.
func coversTheSource(step models.ScenarioStep, translation models.ScenarioStepTranslation) bool {
	pairs := [][2]string{
		{step.Title, translation.Title},
		{step.TextContent, translation.TextContent},
		{step.HintContent, translation.HintContent},
	}
	for _, pair := range pairs {
		if pair[0] != "" && pair[1] == "" {
			return false
		}
	}
	return true
}

// StepSourceHash fingerprints the prose a translation was written against.
//
// Only the translatable fields go in. Editing a verify script must not mark a
// translation stale — the script is shared by every language and says nothing
// about the wording. Editing the text must, because that is precisely the
// silent rot this guards: the French still reads correctly while describing
// something that has changed.
func StepSourceHash(step models.ScenarioStep) string {
	sum := sha256.New()
	for _, field := range []string{step.Title, step.TextContent, step.HintContent} {
		sum.Write([]byte(field))
		sum.Write([]byte{0}) // separator: "ab"+"c" must not hash as "a"+"bc"
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// TranslationCoverage reports every locale the scenario declares.
func TranslationCoverage(db *gorm.DB, scenarioID uuid.UUID) ([]LocaleCoverage, error) {
	var scenario models.Scenario
	if err := db.First(&scenario, "id = ?", scenarioID).Error; err != nil {
		return nil, err
	}

	locales, err := scenario.GetLocales()
	if err != nil {
		return nil, err
	}
	if len(locales) == 0 {
		return nil, nil
	}

	var steps []models.ScenarioStep
	if err := db.Where("scenario_id = ?", scenarioID).Order("\"order\" ASC").Find(&steps).Error; err != nil {
		return nil, err
	}

	var translations []models.ScenarioStepTranslation
	if len(steps) > 0 {
		stepIDs := make([]uuid.UUID, len(steps))
		for i, step := range steps {
			stepIDs[i] = step.ID
		}
		if err := db.Where("step_id IN ?", stepIDs).Find(&translations).Error; err != nil {
			return nil, err
		}
	}

	byLocaleAndStep := make(map[string]map[uuid.UUID]models.ScenarioStepTranslation, len(locales))
	for _, translation := range translations {
		perStep, ok := byLocaleAndStep[translation.Locale]
		if !ok {
			perStep = map[uuid.UUID]models.ScenarioStepTranslation{}
			byLocaleAndStep[translation.Locale] = perStep
		}
		perStep[translation.StepID] = translation
	}

	report := make([]LocaleCoverage, 0, len(locales))
	for _, locale := range locales {
		report = append(report, coverageForLocale(scenario, steps, byLocaleAndStep[locale], locale))
	}
	return report, nil
}

func coverageForLocale(
	scenario models.Scenario,
	steps []models.ScenarioStep,
	translated map[uuid.UUID]models.ScenarioStepTranslation,
	locale string,
) LocaleCoverage {
	coverage := LocaleCoverage{
		Locale:     locale,
		TotalSteps: len(steps),
		Steps:      make([]StepTranslationState, 0, len(steps)),
	}

	// The default locale is the content itself. There is nothing to translate
	// and nothing that can go stale against it.
	if locale == scenario.DefaultLocale {
		coverage.Translated = len(steps)
		coverage.Complete = true
		for _, step := range steps {
			coverage.Steps = append(coverage.Steps, StepTranslationState{
				StepID: step.ID, Order: step.Order, State: StateTranslated,
			})
		}
		return coverage
	}

	for _, step := range steps {
		state := StateMissing
		if translation, ok := translated[step.ID]; ok {
			coverage.Translated++
			switch {
			// An empty hash is not proof of freshness, it is the absence of
			// proof. Reporting it as current would tell a trainer a translation
			// is up to date when nothing ever checked.
			case translation.SourceHash != StepSourceHash(step):
				state = StateStale
				coverage.Stale++
			case !coversTheSource(step, translation):
				state = StatePartial
				coverage.Partial++
			default:
				state = StateTranslated
			}
		} else {
			coverage.Missing++
		}
		coverage.Steps = append(coverage.Steps, StepTranslationState{
			StepID: step.ID, Order: step.Order, State: state,
		})
	}

	coverage.Complete = coverage.Missing == 0 && coverage.Stale == 0 && coverage.Partial == 0
	return coverage
}

// LaunchableLocales names the languages a learner may actually start this
// scenario in.
//
// Derived from the coverage report rather than from the declaration, so a
// locale withdraws itself the moment its source is edited — nobody has to
// remember to un-offer it. That is the difference this whole mechanism buys:
// an out-of-date translation stops being offered instead of quietly shipping.
//
// Empty means the scenario is single-language and a launcher should show no
// picker at all, rather than one with a single entry.
func LaunchableLocales(db *gorm.DB, scenarioID uuid.UUID) ([]string, error) {
	coverage, err := TranslationCoverage(db, scenarioID)
	if err != nil {
		return nil, err
	}

	// A language whose vocabulary is incomplete cannot be offered however well
	// its prose reads. Untranslated prose is English in a French world —
	// awkward, still playable. An unnamed room means the setup script cannot
	// build the world at all, and the learner gets an empty container.
	unnamed, err := localesMissingVocabulary(db, scenarioID)
	if err != nil {
		return nil, err
	}

	launchable := make([]string, 0, len(coverage))
	for _, locale := range coverage {
		if locale.Complete && !unnamed[locale.Locale] {
			launchable = append(launchable, locale.Locale)
		}
	}
	if len(launchable) == 0 {
		return nil, nil
	}
	return launchable, nil
}

// localesMissingVocabulary names the languages that cannot build the world.
//
// Derived from the same validation the editor shows, so "the editor says this
// is fine" and "the launcher offers it" cannot disagree.
func localesMissingVocabulary(db *gorm.DB, scenarioID uuid.UUID) (map[string]bool, error) {
	problems, err := ValidateLexicon(db, scenarioID)
	if err != nil {
		return nil, err
	}

	missing := map[string]bool{}
	for _, problem := range problems {
		if locale, _, found := strings.Cut(problem, ":"); found {
			missing[locale] = true
		}
	}
	return missing, nil
}
