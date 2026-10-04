package scenarios_test

// An imported or seeded scenario used to keep every script and text twice:
// inline on the row and in a ProjectFile the row pointed at, and the runtime
// preferred the file. The editor writes the inline field, so a teacher's edit
// changed nothing the learner saw. These tests drive the editor's real routes
// and then read what the runtime and the export read.

import (
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

const importedContentAuthor = "imported-content-teacher"

func seedImportedScenario(t *testing.T, db *gorm.DB) (*models.Scenario, *models.ScenarioStep) {
	t.Helper()
	scenario, _, err := services.NewScenarioSeedService(db).SeedScenario(dto.SeedScenarioInput{
		Title:       "Imported Content Editing",
		IntroText:   "imported intro",
		FinishText:  "imported finish",
		SetupScript: "echo imported setup",
		Steps: []dto.SeedStepInput{{
			Title:            "Step one",
			TextContent:      "imported text",
			HintContent:      "### Hint 1\nimported first\n### Hint 2\nimported second",
			VerifyScript:     "echo imported verify",
			BackgroundScript: "echo imported background",
			ForegroundScript: "echo imported foreground",
		}},
	}, importedContentAuthor, nil)
	require.NoError(t, err)

	var step models.ScenarioStep
	require.NoError(t, db.First(&step, "scenario_id = ?", scenario.ID).Error)
	return reloadScenario(t, db, scenario.ID), &step
}

func hintContents(t *testing.T, db *gorm.DB, stepID uuid.UUID) []string {
	t.Helper()
	var hints []models.ScenarioStepHint
	require.NoError(t, db.Where("step_id = ?", stepID).Order("level ASC").Find(&hints).Error)
	contents := make([]string, len(hints))
	for i, h := range hints {
		contents[i] = h.Content
	}
	return contents
}

func TestImportedStep_PatchContent_RuntimeAndExportUseTheEdit(t *testing.T) {
	db := freshTestDB(t)
	scenario, step := seedImportedScenario(t, db)
	router := setupArchiveRouter(t, db, importedContentAuthor, []string{"member"})

	resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+step.ID.String(), map[string]any{
		"text_content":      "edited text",
		"verify_script":     "echo edited verify",
		"background_script": "echo edited background",
		"foreground_script": "echo edited foreground",
	})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())

	assert.Equal(t, "edited text", services.ResolveStepText(db, *reloadStep(t, db, step.ID), "").Text)
	exported, err := services.NewScenarioExportService(db).ExportAsJSON(scenario.ID)
	require.NoError(t, err)
	require.Len(t, exported.Steps, 1)
	assert.Equal(t, "edited text", exported.Steps[0].TextContent)
	assert.Equal(t, "echo edited verify", exported.Steps[0].VerifyScript)
	assert.Equal(t, "echo edited background", exported.Steps[0].BackgroundScript)
	assert.Equal(t, "echo edited foreground", exported.Steps[0].ForegroundScript)
}

func TestImportedStep_PatchTitleOnly_KeepsImportedContent(t *testing.T) {
	db := freshTestDB(t)
	_, step := seedImportedScenario(t, db)
	router := setupArchiveRouter(t, db, importedContentAuthor, []string{"member"})

	resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+step.ID.String(),
		map[string]any{"title": "Renamed"})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())

	assert.Equal(t, "imported text", services.ResolveStepText(db, *reloadStep(t, db, step.ID), "").Text)
	assert.Equal(t, []string{"imported first", "imported second"}, hintContents(t, db, step.ID))
}

func TestImportedScenario_PatchProse_RuntimeUsesTheEdit(t *testing.T) {
	db := freshTestDB(t)
	scenario, _ := seedImportedScenario(t, db)
	router := setupArchiveRouter(t, db, importedContentAuthor, []string{"member"})

	resp := sendJSON(t, router, http.MethodPatch, "/scenarios/"+scenario.ID.String(), map[string]any{
		"intro_text":   "edited intro",
		"finish_text":  "edited finish",
		"setup_script": "echo edited setup",
	})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())

	prose := services.ResolveScenarioText(db, *reloadScenario(t, db, scenario.ID), "")
	assert.Equal(t, "edited intro", prose.Intro)
	assert.Equal(t, "edited finish", prose.Finish)
	exported, err := services.NewScenarioExportService(db).ExportAsJSON(scenario.ID)
	require.NoError(t, err)
	assert.Equal(t, "echo edited setup", exported.SetupScript)
}

func TestImportedStep_PatchHint_RebuildsProgressiveHints(t *testing.T) {
	db := freshTestDB(t)
	_, step := seedImportedScenario(t, db)
	require.Equal(t, []string{"imported first", "imported second"}, hintContents(t, db, step.ID))
	router := setupArchiveRouter(t, db, importedContentAuthor, []string{"member"})

	resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+step.ID.String(), map[string]any{
		"hint_content": "### Hint 1\nedited first\n### Hint 2\nedited second\n### Hint 3\nedited third",
	})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())
	assert.Equal(t, []string{"edited first", "edited second", "edited third"}, hintContents(t, db, step.ID),
		"the hints a learner reveals are the rows, so they must follow the edited hint")

	resp = sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+step.ID.String(),
		map[string]any{"hint_content": ""})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())
	assert.Empty(t, hintContents(t, db, step.ID), "clearing the hint removes every progressive hint")
}

// A save resending the stored hint leaves the rows alone, including rows an
// administrator edited one by one.
func TestImportedStep_PatchSameHint_KeepsHintRows(t *testing.T) {
	db := freshTestDB(t)
	_, step := seedImportedScenario(t, db)
	require.NoError(t, db.Model(&models.ScenarioStepHint{}).Where("step_id = ? AND level = 1", step.ID).
		Update("content", "hand-edited first").Error)
	router := setupArchiveRouter(t, db, importedContentAuthor, []string{"member"})

	resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+step.ID.String(),
		map[string]any{"title": "Renamed", "hint_content": step.HintContent})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())
	assert.Equal(t, []string{"hand-edited first", "imported second"}, hintContents(t, db, step.ID))
}

type rejectStepUpdateHook struct{ hooks.BaseHook }

func (rejectStepUpdateHook) Execute(*hooks.HookContext) error { return errors.New("rejected") }

// A PATCH that fails after the hint sync ran must not leave the hint rows
// rebuilt from a hint_content that was never saved.
func TestImportedStep_PatchHintRejected_KeepsHintRows(t *testing.T) {
	db := freshTestDB(t)
	_, step := seedImportedScenario(t, db)
	router := setupArchiveRouter(t, db, importedContentAuthor, []string{"member"})
	require.NoError(t, hooks.GlobalHookRegistry.RegisterHook(&rejectStepUpdateHook{hooks.BaseHook{
		Name: "test_reject_step_update", EntityName: "ScenarioStep",
		HookTypes: []hooks.HookType{hooks.BeforeUpdate}, Enabled: true, Priority: 1000,
	}}))

	resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+step.ID.String(),
		map[string]any{"hint_content": "### Hint 1\nnever saved"})
	require.NotEqual(t, http.StatusNoContent, resp.Code)

	assert.Equal(t, step.HintContent, reloadStep(t, db, step.ID).HintContent)
	assert.Equal(t, []string{"imported first", "imported second"}, hintContents(t, db, step.ID))
}
