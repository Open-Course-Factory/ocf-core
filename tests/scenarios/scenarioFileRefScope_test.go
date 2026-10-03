// Scripts and texts live inline only. The deprecated file-id columns are not
// part of the API any more, so no write can point a scenario or a step at a
// file — its own or another organisation's (the #516 concern).
package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
)

func sendJSON(t *testing.T, router *gin.Engine, method, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewBuffer(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func reloadStep(t *testing.T, db *gorm.DB, id uuid.UUID) *models.ScenarioStep {
	t.Helper()
	var s models.ScenarioStep
	require.NoError(t, db.First(&s, "id = ?", id).Error)
	return &s
}

func reloadScenario(t *testing.T, db *gorm.DB, id uuid.UUID) *models.Scenario {
	t.Helper()
	var s models.Scenario
	require.NoError(t, db.First(&s, "id = ?", id).Error)
	return &s
}

// Even a platform administrator, whom no ownership rule binds, cannot set them.
func TestFileRefColumns_NotWritableThroughTheAPI(t *testing.T) {
	db := freshTestDB(t)
	file := models.ProjectFile{Name: "secret.sh", ContentType: "script", Content: "echo secret", StorageType: "database"}
	require.NoError(t, db.Create(&file).Error)
	scenario := models.Scenario{Name: "file-refs", Title: "Refs", InstanceType: "ubuntu:22.04", SourceType: "seed", CreatedByID: "owner"}
	require.NoError(t, db.Create(&scenario).Error)
	step := models.ScenarioStep{ScenarioID: scenario.ID, Order: 1, Title: "Step", StepType: "terminal"}
	require.NoError(t, db.Create(&step).Error)
	router := setupArchiveRouter(t, db, "platform-admin", []string{"administrator"})

	stepBody := map[string]any{"title": "Renamed"}
	for _, column := range []string{"verify_script_id", "background_script_id", "foreground_script_id", "text_file_id", "hint_file_id"} {
		stepBody[column] = file.ID.String()
	}
	resp := sendJSON(t, router, http.MethodPatch, "/scenario-steps/"+step.ID.String(), stepBody)
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())
	storedStep := reloadStep(t, db, step.ID)
	assert.Equal(t, "Renamed", storedStep.Title)
	assert.Nil(t, storedStep.VerifyScriptID)
	assert.Nil(t, storedStep.BackgroundScriptID)
	assert.Nil(t, storedStep.ForegroundScriptID)
	assert.Nil(t, storedStep.TextFileID)
	assert.Nil(t, storedStep.HintFileID)

	resp = sendJSON(t, router, http.MethodPatch, "/scenarios/"+scenario.ID.String(), map[string]any{
		"title": "Renamed", "setup_script_id": file.ID.String(), "intro_file_id": file.ID.String(), "finish_file_id": file.ID.String(),
	})
	require.Equal(t, http.StatusNoContent, resp.Code, "body=%s", resp.Body.String())
	stored := reloadScenario(t, db, scenario.ID)
	assert.Nil(t, stored.SetupScriptID)
	assert.Nil(t, stored.IntroFileID)
	assert.Nil(t, stored.FinishFileID)

	resp = sendJSON(t, router, http.MethodPost, "/scenario-steps", map[string]any{
		"scenario_id": scenario.ID.String(), "order": 2, "title": "Created", "step_type": "terminal",
		"verify_script_id": file.ID.String(),
	})
	require.Equal(t, http.StatusCreated, resp.Code, "body=%s", resp.Body.String())
	var created models.ScenarioStep
	require.NoError(t, db.First(&created, "scenario_id = ? AND title = ?", scenario.ID, "Created").Error)
	assert.Nil(t, created.VerifyScriptID)
}
