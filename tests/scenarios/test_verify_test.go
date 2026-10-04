package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
)

const testVerifyAuthor = "author-test-verify"

// testVerifyFixture is a scenario authored by testVerifyAuthor and one session
// on it, with the step progress row a real run would have.
type testVerifyFixture struct {
	session  models.ScenarioSession
	execHits *[]map[string]any
}

// newTestVerifyFixture seeds the scenario and a session owned by owner, and
// points tt-backend at a fake whose /1.0/exec answers with exitCode and the
// given streams, recording each request body.
func newTestVerifyFixture(t *testing.T, db *gorm.DB, owner string, isPreview bool, exitCode int, stdout, stderr string) testVerifyFixture {
	t.Helper()

	scenario := models.Scenario{
		Name: "test-verify", Title: "Test verify", InstanceType: "ubuntu:22.04",
		CreatedByID: testVerifyAuthor,
	}
	require.NoError(t, db.Create(&scenario).Error)
	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step", VerifyScript: "#!/bin/bash\ntest -f /saved",
	}).Error)

	terminalID := "terminal-test-verify"
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: owner, CurrentStep: 0, Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalID, IsPreview: isPreview,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{
		SessionID: session.ID, StepOrder: 0, Status: "active",
	}).Error)

	hits := []map[string]any{}
	tt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path == "/1.0/exec" {
			hits = append(hits, body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"exit_code": exitCode, "stdout": stdout, "stderr": stderr})
	}))
	t.Cleanup(tt.Close)
	configureTTServerForLaunch(t, tt.URL)

	return testVerifyFixture{session: session, execHits: &hits}
}

func postTestVerify(t *testing.T, db *gorm.DB, caller string, sessionID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", caller)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	controller := scenarioController.NewScenarioProgressControllerWithTerminalService(db, newMockTTService())
	api.POST("/scenario-sessions/:id/test-verify", controller.TestVerifyScript)

	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/"+sessionID+"/test-verify", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func TestTestVerifyScript_AuthorPreview_RunsScriptAndReportsResult(t *testing.T) {
	db := freshTestDB(t)
	f := newTestVerifyFixture(t, db, testVerifyAuthor, true, 0, "all good", "")

	w := postTestVerify(t, db, testVerifyAuthor, f.session.ID.String(), dto.TestVerifyScriptInput{Script: "#!/bin/bash\ntest -d /tmp"})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp dto.TestVerifyScriptResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Passed)
	assert.Equal(t, 0, resp.ExitCode)
	assert.Equal(t, "all good", resp.Output)
	assert.GreaterOrEqual(t, resp.DurationMs, int64(0))

	require.Len(t, *f.execHits, 1, "the candidate script must reach the container exactly once")
	hit := (*f.execHits)[0]
	assert.Equal(t, "terminal-test-verify", hit["session_id"])
	assert.Equal(t, []any{"/bin/bash", "-c", "#!/bin/bash\ntest -d /tmp"}, hit["command"],
		"the candidate runs, not the saved verify script, through the same shebang handling as a real verify")
}

func TestTestVerifyScript_FailingScript_ReportsExitCodeAndOutput(t *testing.T) {
	db := freshTestDB(t)
	f := newTestVerifyFixture(t, db, testVerifyAuthor, true, 3, "partial", "missing /etc/foo")

	w := postTestVerify(t, db, testVerifyAuthor, f.session.ID.String(), dto.TestVerifyScriptInput{Script: "test -f /etc/foo"})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp dto.TestVerifyScriptResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Passed)
	assert.Equal(t, 3, resp.ExitCode)
	assert.Equal(t, "partial\nmissing /etc/foo", resp.Output)
}

func TestTestVerifyScript_PassingScript_LeavesSessionUntouched(t *testing.T) {
	db := freshTestDB(t)
	f := newTestVerifyFixture(t, db, testVerifyAuthor, true, 0, "", "")

	w := postTestVerify(t, db, testVerifyAuthor, f.session.ID.String(), dto.TestVerifyScriptInput{Script: "true"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var session models.ScenarioSession
	require.NoError(t, db.First(&session, "id = ?", f.session.ID).Error)
	assert.Equal(t, 0, session.CurrentStep, "a passing test must not advance the session")
	assert.Equal(t, "active", session.Status)

	var progress []models.ScenarioStepProgress
	require.NoError(t, db.Where("session_id = ?", f.session.ID).Find(&progress).Error)
	require.Len(t, progress, 1, "no progress row may be created for the next step")
	assert.Equal(t, "active", progress[0].Status)
	assert.Equal(t, 0, progress[0].VerifyAttempts, "a test run is not a verify attempt")
}

func TestTestVerifyScript_Refusals(t *testing.T) {
	cases := []struct {
		name      string
		owner     string
		caller    string
		isPreview bool
		script    string
		want      int
	}{
		{"caller does not own the session", testVerifyAuthor, "someone-else", true, "true", http.StatusForbidden},
		{"session is a learner run, not a preview", testVerifyAuthor, testVerifyAuthor, false, "true", http.StatusForbidden},
		{"previewer cannot manage the scenario", "teacher-not-author", "teacher-not-author", true, "true", http.StatusForbidden},
		{"script is empty", testVerifyAuthor, testVerifyAuthor, true, "", http.StatusBadRequest},
		{"script exceeds the size cap", testVerifyAuthor, testVerifyAuthor, true, strings.Repeat("x", 64*1024+1), http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			f := newTestVerifyFixture(t, db, tc.owner, tc.isPreview, 0, "", "")

			w := postTestVerify(t, db, tc.caller, f.session.ID.String(), dto.TestVerifyScriptInput{Script: tc.script})

			assert.Equal(t, tc.want, w.Code, w.Body.String())
			assert.Empty(t, *f.execHits, "a refused request must never reach the container")
		})
	}
}
