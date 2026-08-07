package scenarios_test

import (
	"reflect"
	"strings"
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
// is still expected to differ on, each with the reason it does. Scalar fields
// outside this map must come through unchanged; ProjectFile references are
// handled by isProjectFileRef below, which asserts re-pointing rather than
// equality.
var stepFieldsNotCopiedVerbatim = map[string]string{
	"ScenarioID": "points at the new scenario by definition",
	"Hints":      "separate rows with their own IDs — compared by content below",
	"Questions":  "separate rows with their own IDs — compared by content below",
}

// isProjectFileRef reports whether a field is a reference to a ProjectFile.
// Duplication re-points these at freshly copied files instead of sharing the
// originals, so they are asserted to have been re-pointed rather than compared.
//
// Matching on the name suffix rather than an explicit list is deliberate: new
// ProjectFile references keep arriving (the effect-asset work adds two more),
// and this way they are covered on arrival instead of silently skipped.
func isProjectFileRef(field reflect.StructField) bool {
	if field.Type != reflect.TypeOf((*uuid.UUID)(nil)) {
		return false
	}
	return strings.HasSuffix(field.Name, "FileID") || strings.HasSuffix(field.Name, "ScriptID")
}

func TestDuplicateScenario_StepsAreFieldCompleteAgainstSource(t *testing.T) {
	db := freshTestDB(t)
	source := createFullSourceScenario(t, db, nil)

	// The source fixture is generic, so give its steps the values whose loss
	// motivated this test: a quiz step with questions, and an exam-mode step.
	require.NoError(t, db.Model(&models.ScenarioStep{}).
		Where("id = ?", source.Steps[0].ID).
		Updates(map[string]any{"step_type": "quiz", "show_immediate_feedback": true}).Error)
	require.NoError(t, db.Model(&models.ScenarioStep{}).
		Where("id = ?", source.Steps[1].ID).
		Updates(map[string]any{"step_type": "info", "show_immediate_feedback": false}).Error)
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
			if isProjectFileRef(field) {
				assertProjectFileRefRepointed(t, field.Name, src.Field(f), dst.Field(f))
				continue
			}

			assert.Equal(t, src.Field(f).Interface(), dst.Field(f).Interface(),
				"step %d: field %s was not carried over by duplication. Add it to the copy in scenarioDuplicateService.DuplicateScenario, or to stepFieldsNotCopiedVerbatim with the reason it legitimately differs.",
				sourceSteps[i].Order, field.Name)
		}
	}
}

// assertProjectFileRefRepointed checks that a ProjectFile reference was copied
// as a reference: present when the source had one, absent when it did not, and
// pointing at a different file. Sharing the original would make deleting one
// scenario break the other.
func assertProjectFileRefRepointed(t *testing.T, fieldName string, srcField, dstField reflect.Value) {
	t.Helper()

	srcRef := srcField.Interface().(*uuid.UUID)
	dstRef := dstField.Interface().(*uuid.UUID)

	if srcRef == nil {
		assert.Nil(t, dstRef, "%s: the source had no file, so the copy must not invent one", fieldName)
		return
	}
	if !assert.NotNil(t, dstRef, "%s: the source referenced a file and the copy lost it", fieldName) {
		return
	}
	assert.NotEqual(t, *srcRef, *dstRef,
		"%s: the copy shares the source's ProjectFile — deleting either scenario would break the other", fieldName)
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
