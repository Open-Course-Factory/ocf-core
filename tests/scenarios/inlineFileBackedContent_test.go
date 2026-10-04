package scenarios_test

// Pins InlineFileBackedContent, the startup migration that ends the double
// storage of scenario scripts and texts: a file a row points at is copied into
// the inline column (the file is what learners ran), the pointer is cleared,
// and the file is deleted unless something else still needs it.

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/initialization"
	"soli/formations/src/scenarios/models"
)

func createContentFile(t *testing.T, db *gorm.DB, contentType, content string, scenarioID *uuid.UUID) uuid.UUID {
	t.Helper()
	f := models.ProjectFile{Name: "f", ContentType: contentType, Content: content, StorageType: "database", ScenarioID: scenarioID}
	require.NoError(t, db.Create(&f).Error)
	return f.ID
}

func fileExists(t *testing.T, db *gorm.DB, id uuid.UUID) bool {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&models.ProjectFile{}).Where("id = ?", id).Count(&count).Error)
	return count > 0
}

func TestInlineFileBackedContent_FileWinsOverDivergedInline(t *testing.T) {
	db := freshTestDB(t)
	setup := createContentFile(t, db, "script", "echo setup from file", nil)
	intro := createContentFile(t, db, "markdown", "intro from file", nil)
	finish := createContentFile(t, db, "markdown", "finish from file", nil)
	scenario := models.Scenario{
		Name: "inline-migration", Title: "Inline", InstanceType: "ubuntu:22.04", SourceType: "seed",
		SetupScript: "echo stale setup", IntroText: "stale intro", FinishText: "",
		SetupScriptID: &setup, IntroFileID: &intro, FinishFileID: &finish,
	}
	require.NoError(t, db.Create(&scenario).Error)

	verify := createContentFile(t, db, "script", "echo verify from file", nil)
	background := createContentFile(t, db, "script", "echo background from file", nil)
	foreground := createContentFile(t, db, "script", "echo foreground from file", nil)
	text := createContentFile(t, db, "markdown", "text from file", nil)
	hint := createContentFile(t, db, "markdown", "hint from file", nil)
	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step", StepType: "terminal",
		VerifyScript: "echo stale verify", TextContent: "stale text",
		VerifyScriptID: &verify, BackgroundScriptID: &background, ForegroundScriptID: &foreground,
		TextFileID: &text, HintFileID: &hint,
	}
	require.NoError(t, db.Create(&step).Error)

	initialization.InlineFileBackedContent(db)

	stored := reloadScenario(t, db, scenario.ID)
	assert.Equal(t, "echo setup from file", stored.SetupScript)
	assert.Equal(t, "intro from file", stored.IntroText)
	assert.Equal(t, "finish from file", stored.FinishText)
	assert.Nil(t, stored.SetupScriptID)
	assert.Nil(t, stored.IntroFileID)
	assert.Nil(t, stored.FinishFileID)

	storedStep := reloadStep(t, db, step.ID)
	assert.Equal(t, "echo verify from file", storedStep.VerifyScript)
	assert.Equal(t, "echo background from file", storedStep.BackgroundScript)
	assert.Equal(t, "echo foreground from file", storedStep.ForegroundScript)
	assert.Equal(t, "text from file", storedStep.TextContent)
	assert.Equal(t, "hint from file", storedStep.HintContent)
	assert.Nil(t, storedStep.VerifyScriptID)
	assert.Nil(t, storedStep.BackgroundScriptID)
	assert.Nil(t, storedStep.ForegroundScriptID)
	assert.Nil(t, storedStep.TextFileID)
	assert.Nil(t, storedStep.HintFileID)

	for _, id := range []uuid.UUID{setup, intro, finish, verify, background, foreground, text, hint} {
		assert.False(t, fileExists(t, db, id), "a file nothing points at any more is deleted")
	}
}

// A pointer at a missing or deleted file served the inline copy at runtime, so
// the inline copy is what stays.
func TestInlineFileBackedContent_DanglingPointer_KeepsInline(t *testing.T) {
	db := freshTestDB(t)
	missing := uuid.New()
	deleted := createContentFile(t, db, "script", "echo deleted file", nil)
	require.NoError(t, db.Delete(&models.ProjectFile{}, "id = ?", deleted).Error)
	scenario := models.Scenario{
		Name: "inline-dangling", Title: "Dangling", InstanceType: "ubuntu:22.04", SourceType: "seed",
		IntroText: "inline intro", IntroFileID: &missing,
	}
	require.NoError(t, db.Create(&scenario).Error)
	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step", StepType: "terminal",
		VerifyScript: "echo inline verify", VerifyScriptID: &deleted,
	}
	require.NoError(t, db.Create(&step).Error)

	initialization.InlineFileBackedContent(db)

	stored := reloadScenario(t, db, scenario.ID)
	assert.Equal(t, "inline intro", stored.IntroText)
	assert.Nil(t, stored.IntroFileID)
	storedStep := reloadStep(t, db, step.ID)
	assert.Equal(t, "echo inline verify", storedStep.VerifyScript)
	assert.Nil(t, storedStep.VerifyScriptID)
}

func TestInlineFileBackedContent_FileSharedByTwoColumns_CopiedIntoBoth(t *testing.T) {
	db := freshTestDB(t)
	scenario := models.Scenario{Name: "inline-shared", Title: "Shared", InstanceType: "ubuntu:22.04", SourceType: "seed"}
	require.NoError(t, db.Create(&scenario).Error)
	shared := createContentFile(t, db, "markdown", "shared text", nil)
	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step", StepType: "terminal",
		TextFileID: &shared, HintFileID: &shared,
	}
	require.NoError(t, db.Create(&step).Error)

	initialization.InlineFileBackedContent(db)

	storedStep := reloadStep(t, db, step.ID)
	assert.Equal(t, "shared text", storedStep.TextContent)
	assert.Equal(t, "shared text", storedStep.HintContent)
	assert.False(t, fileExists(t, db, shared))
}

func TestInlineFileBackedContent_KeepsImagesAndFilesItDidNotInline(t *testing.T) {
	db := freshTestDB(t)
	scenario := models.Scenario{Name: "inline-images", Title: "Images", InstanceType: "ubuntu:22.04", SourceType: "seed"}
	require.NoError(t, db.Create(&scenario).Error)
	image := createContentFile(t, db, "image", "aW1hZ2U=", &scenario.ID)
	pointedImage := createContentFile(t, db, "image", "aW1hZ2U=", nil)
	linkedScript := createContentFile(t, db, "script", "echo linked", &scenario.ID)
	unreferenced := createContentFile(t, db, "script", "echo uploaded by an admin", nil)
	step := models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step", StepType: "terminal",
		TextFileID: &pointedImage, VerifyScriptID: &linkedScript,
	}
	require.NoError(t, db.Create(&step).Error)

	initialization.InlineFileBackedContent(db)

	assert.True(t, fileExists(t, db, image))
	assert.True(t, fileExists(t, db, pointedImage), "an image is never deleted")
	assert.True(t, fileExists(t, db, linkedScript), "a file linked to its scenario is still listed with it")
	assert.True(t, fileExists(t, db, unreferenced), "only the files the pointers held are collected")
	assert.Nil(t, reloadStep(t, db, step.ID).TextFileID)
}

// Translations are separate rows overlaid on the inline text; inlining the base
// text must not touch them.
func TestInlineFileBackedContent_KeepsTranslations(t *testing.T) {
	db := freshTestDB(t)
	scenario := models.Scenario{Name: "inline-translations", Title: "T", InstanceType: "ubuntu:22.04", SourceType: "seed"}
	require.NoError(t, db.Create(&scenario).Error)
	text := createContentFile(t, db, "markdown", "english text", nil)
	step := models.ScenarioStep{ScenarioID: scenario.ID, Order: 0, Title: "Step", StepType: "terminal", TextFileID: &text}
	require.NoError(t, db.Create(&step).Error)
	require.NoError(t, db.Create(&models.ScenarioStepTranslation{StepID: step.ID, Locale: "fr", TextContent: "texte français"}).Error)

	initialization.InlineFileBackedContent(db)
	initialization.InlineFileBackedContent(db) // idempotent

	storedStep := reloadStep(t, db, step.ID)
	assert.Equal(t, "english text", storedStep.TextContent)
	var translation models.ScenarioStepTranslation
	require.NoError(t, db.First(&translation, "step_id = ? AND locale = ?", step.ID, "fr").Error)
	assert.Equal(t, "texte français", translation.TextContent)
}

// The migration says what it changed, and nothing once there is nothing left.
func TestInlineFileBackedContent_LogsOnlyWhatItChanged(t *testing.T) {
	db := freshTestDB(t)
	scenario := models.Scenario{Name: "inline-log", Title: "Log", InstanceType: "ubuntu:22.04", SourceType: "seed"}
	require.NoError(t, db.Create(&scenario).Error)
	shared := createContentFile(t, db, "markdown", "shared text", nil)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step", StepType: "terminal",
		TextFileID: &shared, HintFileID: &shared,
	}).Error)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	initialization.InlineFileBackedContent(db)

	assert.Contains(t, logs.String(), "[INLINE-CONTENT-MIGRATION] 1 scenario_steps.text_file_id pointers cleared")
	assert.Contains(t, logs.String(), "[INLINE-CONTENT-MIGRATION] 1 scenario_steps.hint_file_id pointers cleared")
	assert.Contains(t, logs.String(), "[INLINE-CONTENT-MIGRATION] 1 content files soft-deleted")
	assert.Equal(t, 3, strings.Count(logs.String(), "[INLINE-CONTENT-MIGRATION]"))

	logs.Reset()
	initialization.InlineFileBackedContent(db)
	assert.NotContains(t, logs.String(), "[INLINE-CONTENT-MIGRATION]")
}
