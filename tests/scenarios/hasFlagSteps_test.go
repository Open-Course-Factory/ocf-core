package scenarios_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
)

// A learner's validated-flags panel shows for any scenario with a flag step,
// not only one imported with flags_enabled: an editor-built flag scenario has
// it false. has_flag_steps survives the redaction that strips the steps.

func createPublicScenarioWithStep(t *testing.T, name, stepType string) *models.Scenario {
	t.Helper()
	scenario := &models.Scenario{
		Name: name, Title: name, InstanceType: "ubuntu:22.04", CreatedByID: "author-1", IsPublic: true,
	}
	require.NoError(t, sharedTestDB.Create(scenario).Error)
	require.False(t, scenario.FlagsEnabled)
	require.NoError(t, sharedTestDB.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "s", StepType: stepType,
	}).Error)
	return scenario
}

func TestScenarioOutput_HasFlagSteps_ReachesTheLearner(t *testing.T) {
	db := freshTestDB(t)
	withFlag := createPublicScenarioWithStep(t, "with-flag-step", "flag")
	withoutFlag := createPublicScenarioWithStep(t, "without-flag-step", "terminal")
	router := setupScenarioReadAuthzTest(t, db, "learner-1", []string{"member"}, "/scenarios")

	get := func(path string) []byte {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1"+path, nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		return w.Body.Bytes()
	}

	for id, want := range map[string]bool{withFlag.ID.String(): true, withoutFlag.ID.String(): false} {
		var body map[string]any
		require.NoError(t, json.Unmarshal(get("/scenarios/"+id), &body))
		assert.Nil(t, body["steps"], "the learner still gets no steps")
		assert.Equal(t, want, body["has_flag_steps"], "single GET %s", body["name"])
	}

	var page struct{ Data []map[string]any }
	require.NoError(t, json.Unmarshal(get("/scenarios"), &page))
	require.Len(t, page.Data, 2)
	for _, item := range page.Data {
		assert.Equal(t, item["name"] == "with-flag-step", item["has_flag_steps"], "list GET %s", item["name"])
	}
}
