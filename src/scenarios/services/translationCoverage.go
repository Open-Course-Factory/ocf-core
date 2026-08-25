package services

import (
	"crypto/sha256"
	"encoding/hex"

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
	Complete   bool   `json:"complete"`

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
	StateStale      = "stale"
	StateMissing    = "missing"
)

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
// coverageSource is everything a report is computed from, read in one place so
// that the report itself stays arithmetic rather than arithmetic mixed with I/O.
type coverageSource struct {
	scenario     models.Scenario
	locales      []string
	steps        []models.ScenarioStep
	translations []models.ScenarioStepTranslation
}

// loadCoverageSource reads the scenario, its steps, and the translations
// belonging to those steps.
//
// It stops early twice, and both are about not asking a question whose answer
// is already known: a scenario declaring no language is not translated, and a
// scenario with no steps has nothing for a translation to point at.
func loadCoverageSource(db *gorm.DB, scenarioID uuid.UUID) (coverageSource, error) {
	var source coverageSource

	if err := db.First(&source.scenario, "id = ?", scenarioID).Error; err != nil {
		return source, err
	}

	locales, err := source.scenario.GetLocales()
	if err != nil {
		return source, err
	}
	source.locales = locales
	if len(locales) == 0 {
		return source, nil
	}

	if err := db.Where("scenario_id = ?", scenarioID).Order("\"order\" ASC").Find(&source.steps).Error; err != nil {
		return source, err
	}
	if len(source.steps) == 0 {
		return source, nil
	}

	stepIDs := make([]uuid.UUID, len(source.steps))
	for i, step := range source.steps {
		stepIDs[i] = step.ID
	}
	if err := db.Where("step_id IN ?", stepIDs).Find(&source.translations).Error; err != nil {
		return source, err
	}
	return source, nil
}

// indexByLocaleAndStep arranges the translations so a locale's report can find
// each step's translation directly, rather than walking the whole set per step.
func indexByLocaleAndStep(
	translations []models.ScenarioStepTranslation,
) map[string]map[uuid.UUID]models.ScenarioStepTranslation {
	indexed := map[string]map[uuid.UUID]models.ScenarioStepTranslation{}
	for _, translation := range translations {
		perStep, ok := indexed[translation.Locale]
		if !ok {
			perStep = map[uuid.UUID]models.ScenarioStepTranslation{}
			indexed[translation.Locale] = perStep
		}
		perStep[translation.StepID] = translation
	}
	return indexed
}

func TranslationCoverage(db *gorm.DB, scenarioID uuid.UUID) ([]LocaleCoverage, error) {
	source, err := loadCoverageSource(db, scenarioID)
	if err != nil {
		return nil, err
	}
	if len(source.locales) == 0 {
		return nil, nil
	}

	indexed := indexByLocaleAndStep(source.translations)

	report := make([]LocaleCoverage, 0, len(source.locales))
	for _, locale := range source.locales {
		report = append(report, coverageForLocale(source.scenario, source.steps, indexed[locale], locale))
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
			// An empty hash is not proof of freshness, it is the absence of
			// proof. Reporting it as current would tell a trainer a translation
			// is up to date when nothing ever checked.
			if translation.SourceHash == StepSourceHash(step) {
				state = StateTranslated
			} else {
				state = StateStale
				coverage.Stale++
			}
		} else {
			coverage.Missing++
		}
		coverage.Steps = append(coverage.Steps, StepTranslationState{
			StepID: step.ID, Order: step.Order, State: state,
		})
	}

	coverage.Complete = coverage.Missing == 0 && coverage.Stale == 0
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

	launchable := make([]string, 0, len(coverage))
	for _, locale := range coverage {
		if locale.Complete {
			launchable = append(launchable, locale.Locale)
		}
	}
	if len(launchable) == 0 {
		return nil, nil
	}
	return launchable, nil
}
