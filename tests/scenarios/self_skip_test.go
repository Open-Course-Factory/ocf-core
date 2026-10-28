package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
)

// POST /scenario-sessions/:id/skip-to-step — a trainer moves their own run of
// a lab forward, the way the class view moves a learner's.

const selfSkipAuthor = "author-self-skip"

// selfSkipSession is a two-step scenario written by selfSkipAuthor and an
// active run of it, on step 0, owned by owner.
func selfSkipSession(t *testing.T, db *gorm.DB, owner string) models.ScenarioSession {
	t.Helper()
	scenario := models.Scenario{
		Name: "self-skip-" + owner, Title: "Self skip", InstanceType: "ubuntu:22.04",
		CreatedByID: selfSkipAuthor,
	}
	require.NoError(t, db.Create(&scenario).Error)
	for _, step := range []models.ScenarioStep{
		{ScenarioID: scenario.ID, Order: 0, Title: "Step 1"},
		{ScenarioID: scenario.ID, Order: 1, Title: "Step 2", BackgroundScript: "echo setup", BackgroundTimeoutSeconds: 5},
	} {
		require.NoError(t, db.Create(&step).Error)
	}
	terminalID := "terminal-self-skip-" + owner
	session := models.ScenarioSession{
		ScenarioID: scenario.ID, UserID: owner, CurrentStep: 0, Status: "active",
		StartedAt: time.Now(), TerminalSessionID: &terminalID,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 0, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 1, Status: "locked"}).Error)

	tt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"exit_code": 0})
	}))
	t.Cleanup(tt.Close)
	configureTTServerForLaunch(t, tt.URL)
	return session
}

func postSelfSkip(t *testing.T, db *gorm.DB, caller string, sessionID string, stepOrder int) *httptest.ResponseRecorder {
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
	api.POST("/scenario-sessions/:id/skip-to-step", controller.SkipToStep)

	raw, err := json.Marshal(map[string]int{"step_order": stepOrder})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/"+sessionID+"/skip-to-step", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func currentStepOf(t *testing.T, db *gorm.DB, session models.ScenarioSession) int {
	t.Helper()
	var reloaded models.ScenarioSession
	require.NoError(t, db.First(&reloaded, "id = ?", session.ID).Error)
	return reloaded.CurrentStep
}

func TestSelfSkip_TrainerOwnRun_MovesForward(t *testing.T) {
	db := freshTestDB(t)
	session := selfSkipSession(t, db, selfSkipAuthor)

	w := postSelfSkip(t, db, selfSkipAuthor, session.ID.String(), 1)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, currentStepOf(t, db, session))
}

func TestSelfSkip_LearnerOwnRun_IsRefused(t *testing.T) {
	db := freshTestDB(t)
	session := selfSkipSession(t, db, "learner-self-skip")

	w := postSelfSkip(t, db, "learner-self-skip", session.ID.String(), 1)

	assert.Equal(t, http.StatusForbidden, w.Code, "skipping their own graded run would defeat the grade")
	assert.Equal(t, 0, currentStepOf(t, db, session))
}

func TestSelfSkip_TrainerOnSomeoneElsesRun_IsRefused(t *testing.T) {
	db := freshTestDB(t)
	session := selfSkipSession(t, db, "learner-self-skip")

	w := postSelfSkip(t, db, selfSkipAuthor, session.ID.String(), 1)

	assert.Equal(t, http.StatusForbidden, w.Code, "a learner's run is skipped from the class view, which checks the class")
	assert.Equal(t, 0, currentStepOf(t, db, session))
}
