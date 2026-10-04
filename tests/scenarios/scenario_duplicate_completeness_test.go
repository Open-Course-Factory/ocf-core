package scenarios_test

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// Duplication copies a step's fields by hand, one assignment per column, which
// makes forgetting one silent: the copy saves fine and nothing fails. It has
// already happened twice — StepType and ShowImmediateFeedback were dropped for
// months, turning a duplicated quiz into a terminal step and a duplicated
// exam-mode step into one that reveals its answers.
//
// So this file asserts field-completeness by reflection rather than by listing
// the columns it knows about. A column added tomorrow is compared too, and the
// author either copies it or gets a failing test — no memory of this bug
// required.

// stepFieldsNotCopiedVerbatim are the ScenarioStep fields a faithful duplicate
// is still expected to differ on, each with the reason it does. Every field
// outside this map must come through unchanged.
var stepFieldsNotCopiedVerbatim = map[string]string{
	"ScenarioID": "points at the new scenario by definition",
	"Hints":      "separate rows with their own IDs — compared by content below",
	"Questions":  "separate rows with their own IDs — compared by content below",
}

func TestDuplicateScenario_StepsAreFieldCompleteAgainstSource(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)

	// The source fixture is generic, so give its steps the values whose loss
	// motivated this test: a quiz step with questions, and an exam-mode step.
	// Neither kind carries a flag (models.NormalizeFlagStep).
	require.NoError(t, db.Model(&models.ScenarioStep{}).
		Where("id = ?", source.Steps[0].ID).
		Updates(map[string]any{"step_type": "quiz", "has_flag": false, "show_immediate_feedback": true}).Error)
	require.NoError(t, db.Model(&models.ScenarioStep{}).
		Where("id = ?", source.Steps[1].ID).
		Updates(map[string]any{"step_type": "info", "has_flag": false, "show_immediate_feedback": false}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepQuestion{
		StepID:        source.Steps[0].ID,
		Order:         1,
		QuestionText:  "Which command lists files?",
		QuestionType:  "single_choice",
		Options:       `["ls","cd"]`,
		CorrectAnswer: "ls",
		Explanation:   "ls lists directory contents",
		Points:        2,
	}).Error)

	sourceSteps := loadStepsWithRelations(t, db, source.ID)

	copied, err := services.NewScenarioDuplicateService(db).DuplicateScenario(source.ID, "duplicating-user", nil)
	require.NoError(t, err)
	copiedSteps := loadStepsWithRelations(t, db, copied.ID)

	require.Len(t, copiedSteps, len(sourceSteps), "every source step must be duplicated")

	stepType := reflect.TypeOf(models.ScenarioStep{})
	for i := range sourceSteps {
		src := reflect.ValueOf(sourceSteps[i])
		dst := reflect.ValueOf(copiedSteps[i])

		for f := 0; f < stepType.NumField(); f++ {
			field := stepType.Field(f)

			// The embedded BaseModel holds identity and timestamps, which a
			// copy owns for itself. Skipping by anonymity rather than by field
			// name keeps this correct if BaseModel ever grows a column.
			if field.Anonymous {
				continue
			}
			if _, excluded := stepFieldsNotCopiedVerbatim[field.Name]; excluded {
				continue
			}

			assert.Equal(t, src.Field(f).Interface(), dst.Field(f).Interface(),
				"step %d: field %s was not carried over by duplication. Add it to the copy in scenarioDuplicateService.DuplicateScenario, or to stepFieldsNotCopiedVerbatim with the reason it legitimately differs.",
				sourceSteps[i].Order, field.Name)
		}
	}
}

// loadStepsWithRelations reads a scenario's steps in display order with the
// associations duplication is responsible for reproducing.
func loadStepsWithRelations(t *testing.T, db *gorm.DB, scenarioID uuid.UUID) []models.ScenarioStep {
	t.Helper()
	var steps []models.ScenarioStep
	require.NoError(t, db.
		Preload("Hints", func(db *gorm.DB) *gorm.DB { return db.Order("level ASC") }).
		Preload("Questions", func(db *gorm.DB) *gorm.DB { return db.Order("\"order\" ASC") }).
		Where("scenario_id = ?", scenarioID).
		Order("\"order\" ASC").
		Find(&steps).Error)
	return steps
}

// The two associations excluded from the field sweep still have to survive
// duplication — they are just compared by content, since their rows carry new
// IDs and a new parent.
func TestDuplicateScenario_CopiesStepHintsAndQuestions(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)

	require.NoError(t, db.Model(&models.ScenarioStep{}).
		Where("id = ?", source.Steps[0].ID).
		Update("step_type", "quiz").Error)
	require.NoError(t, db.Create(&models.ScenarioStepQuestion{
		StepID:        source.Steps[0].ID,
		Order:         1,
		QuestionText:  "Which command lists files?",
		QuestionType:  "single_choice",
		Options:       `["ls","cd"]`,
		CorrectAnswer: "ls",
		Explanation:   "ls lists directory contents",
		Points:        2,
	}).Error)

	copied, err := services.NewScenarioDuplicateService(db).DuplicateScenario(source.ID, "duplicating-user", nil)
	require.NoError(t, err)

	sourceSteps := loadStepsWithRelations(t, db, source.ID)
	copiedSteps := loadStepsWithRelations(t, db, copied.ID)
	require.Len(t, copiedSteps, len(sourceSteps))

	for i := range sourceSteps {
		src, dst := sourceSteps[i], copiedSteps[i]

		require.Len(t, dst.Hints, len(src.Hints), "step %d lost hints", src.Order)
		for h := range src.Hints {
			assert.Equal(t, src.Hints[h].Level, dst.Hints[h].Level)
			assert.Equal(t, src.Hints[h].Content, dst.Hints[h].Content)
			assert.Equal(t, dst.ID, dst.Hints[h].StepID, "hint must belong to the copied step")
		}

		require.Len(t, dst.Questions, len(src.Questions),
			"step %d lost quiz questions — a duplicated quiz with no questions renders as an empty exam", src.Order)
		for q := range src.Questions {
			assert.Equal(t, src.Questions[q].Order, dst.Questions[q].Order)
			assert.Equal(t, src.Questions[q].QuestionText, dst.Questions[q].QuestionText)
			assert.Equal(t, src.Questions[q].QuestionType, dst.Questions[q].QuestionType)
			assert.Equal(t, src.Questions[q].Options, dst.Questions[q].Options)
			assert.Equal(t, src.Questions[q].CorrectAnswer, dst.Questions[q].CorrectAnswer)
			assert.Equal(t, src.Questions[q].Explanation, dst.Questions[q].Explanation)
			assert.Equal(t, src.Questions[q].Points, dst.Questions[q].Points)
			assert.Equal(t, dst.ID, dst.Questions[q].StepID, "question must belong to the copied step")
		}
	}
}

// Exam mode is a confidentiality setting: with show_immediate_feedback false
// the API withholds correct answers entirely (core !363). A duplicate that
// silently flipped it back to the zero value handed next year's exam to the
// learners along with its answers.
func TestDuplicateScenario_PreservesExamMode(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)

	require.NoError(t, db.Model(&models.ScenarioStep{}).
		Where("id = ?", source.Steps[0].ID).
		Updates(map[string]any{"step_type": "quiz", "show_immediate_feedback": false}).Error)

	copied, err := services.NewScenarioDuplicateService(db).DuplicateScenario(source.ID, "duplicating-user", nil)
	require.NoError(t, err)

	copiedSteps := loadStepsWithRelations(t, db, copied.ID)
	require.NotEmpty(t, copiedSteps)
	assert.Equal(t, "quiz", copiedSteps[0].StepType, "a duplicated quiz must not come back as a terminal step")
	assert.False(t, copiedSteps[0].ShowImmediateFeedback,
		"a duplicated exam must stay an exam — flipping this on reveals the answers to learners")
}

var scenarioFieldsNotCopiedVerbatim = map[string]string{
	"Name":                    "a fresh unique slug",
	"Title":                   "marked as the copy",
	"FlagSecret":              "the copy must not share the source's flags; minted at its first run",
	"CreatedByID":             "the duplicating user",
	"OrganizationID":          "the organisation the copy is made for",
	"SetupScriptID":           "deprecated file pointer into the source's files; content is inline",
	"IntroFileID":             "deprecated file pointer into the source's files; content is inline",
	"FinishFileID":            "deprecated file pointer into the source's files; content is inline",
	"Steps":                   "separate rows — compared by TestDuplicateScenario_StepsAreFieldCompleteAgainstSource",
	"CompatibleInstanceTypes": "separate rows with their own IDs",
}

// A scenario field the duplicate forgets saves cleanly and fails nobody: a
// copy that dropped Locales silently stopped being offered in French.
func TestDuplicateScenario_ScenarioIsFieldCompleteAgainstSource(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)
	sessionUser := 1000
	require.NoError(t, db.Model(&models.Scenario{}).Where("id = ?", source.ID).Updates(map[string]any{
		"required_features":     `["docker"]`,
		"build_features":        `["network"]`,
		"session_user":          sessionUser,
		"default_locale":        "en",
		"locales":               `["en","fr"]`,
		"allowed_flag_paths":    "/srv",
		"port_exposure_allowed": true,
		"git_repository":        "https://example.org/lab.git",
		"git_branch":            "main",
		"source_path":           "labs/one",
	}).Error)
	var reloaded models.Scenario
	require.NoError(t, db.First(&reloaded, "id = ?", source.ID).Error)

	copied, err := services.NewScenarioDuplicateService(db).DuplicateScenario(source.ID, "duplicating-user", nil)
	require.NoError(t, err)

	scenarioType := reflect.TypeOf(models.Scenario{})
	src, dst := reflect.ValueOf(reloaded), reflect.ValueOf(*copied)
	for f := 0; f < scenarioType.NumField(); f++ {
		field := scenarioType.Field(f)
		if field.Anonymous {
			continue
		}
		if _, excluded := scenarioFieldsNotCopiedVerbatim[field.Name]; excluded {
			continue
		}
		assert.Equal(t, src.Field(f).Interface(), dst.Field(f).Interface(),
			"scenario field %s was not carried over by duplication. Copy it in DuplicateScenario, or list it in scenarioFieldsNotCopiedVerbatim with the reason it differs.",
			field.Name)
	}
}

// A duplicate of a bilingual scenario must stay bilingual: its translations,
// with the source hashes that keep them current, and the lexicon its scripts
// name the world with.
func TestDuplicateScenario_CopiesTranslationsAndLexicon(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)
	require.NoError(t, db.Create(&models.ScenarioTranslation{
		ScenarioID: source.ID, Locale: "fr", Title: "Scénario", Description: "Desc FR",
		Objectives: "Obj FR", Prerequisites: "Pré FR", IntroText: "Intro FR", FinishText: "Fin FR",
	}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepTranslation{
		StepID: source.Steps[0].ID, Locale: "fr", Title: "Étape 1", TextContent: "Texte FR",
		HintContent: "Indice FR", IntroText: "Bienvenue", OutroText: "Bravo", SourceHash: "hash-of-the-english",
	}).Error)
	castle := models.ScenarioLexiconEntry{ScenarioID: source.ID, Key: "CASTLE", Kind: "place", Position: 1,
		Names: []models.ScenarioLexiconName{{Locale: "en", Name: "castle"}, {Locale: "fr", Name: "chateau"}}}
	require.NoError(t, db.Create(&castle).Error)
	cellar := models.ScenarioLexiconEntry{ScenarioID: source.ID, Key: "CELLAR", ParentKey: "CASTLE", Kind: "place", Position: 2,
		Names: []models.ScenarioLexiconName{{Locale: "en", Name: "cellar"}, {Locale: "fr", Name: "cave"}}}
	require.NoError(t, db.Create(&cellar).Error)

	copied, err := services.NewScenarioDuplicateService(db).DuplicateScenario(source.ID, "duplicating-user", nil)
	require.NoError(t, err)

	var translations []models.ScenarioTranslation
	require.NoError(t, db.Where("scenario_id = ?", copied.ID).Find(&translations).Error)
	require.Len(t, translations, 1)
	assert.Equal(t, models.ScenarioTranslation{
		BaseModel: translations[0].BaseModel, ScenarioID: copied.ID, Locale: "fr", Title: "Scénario", Description: "Desc FR",
		Objectives: "Obj FR", Prerequisites: "Pré FR", IntroText: "Intro FR", FinishText: "Fin FR",
	}, translations[0])

	copiedSteps := loadStepsWithRelations(t, db, copied.ID)
	var stepTranslations []models.ScenarioStepTranslation
	require.NoError(t, db.Where("step_id = ?", copiedSteps[0].ID).Find(&stepTranslations).Error)
	require.Len(t, stepTranslations, 1)
	assert.Equal(t, models.ScenarioStepTranslation{
		BaseModel: stepTranslations[0].BaseModel, StepID: copiedSteps[0].ID, Locale: "fr", Title: "Étape 1", TextContent: "Texte FR",
		HintContent: "Indice FR", IntroText: "Bienvenue", OutroText: "Bravo", SourceHash: "hash-of-the-english",
	}, stepTranslations[0], "the source hash keeps the translation current")

	var entries []models.ScenarioLexiconEntry
	require.NoError(t, db.Preload("Names", func(db *gorm.DB) *gorm.DB { return db.Order("locale ASC") }).
		Where("scenario_id = ?", copied.ID).Order("position ASC").Find(&entries).Error)
	require.Len(t, entries, 2)
	for i, want := range []models.ScenarioLexiconEntry{castle, cellar} {
		assert.Equal(t, want.Key, entries[i].Key)
		assert.Equal(t, want.ParentKey, entries[i].ParentKey)
		assert.Equal(t, want.Kind, entries[i].Kind)
		assert.Equal(t, want.Position, entries[i].Position)
		require.Len(t, entries[i].Names, 2)
		for n := range want.Names {
			assert.Equal(t, want.Names[n].Locale, entries[i].Names[n].Locale)
			assert.Equal(t, want.Names[n].Name, entries[i].Names[n].Name)
		}
	}

	var sourceEntries int64
	require.NoError(t, db.Model(&models.ScenarioLexiconEntry{}).Where("scenario_id = ?", source.ID).Count(&sourceEntries).Error)
	assert.EqualValues(t, 2, sourceEntries, "the source keeps its lexicon")
}
