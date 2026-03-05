package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"

	"gorm.io/gorm"
)

func setupTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	// Set userId directly to bypass auth middleware in tests
	api.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-123")
		c.Next()
	})

	controller := scenarioController.NewScenarioController(db)

	// Scenario management routes
	scenarios := api.Group("/scenarios")
	scenarios.POST("/import", controller.ImportScenario)
	scenarios.POST("/:id/start", controller.StartScenario)

	// Session routes
	sessions := api.Group("/scenario-sessions")
	sessions.GET("/:id/current-step", controller.GetCurrentStep)
	sessions.POST("/:id/verify", controller.VerifyStep)
	sessions.POST("/:id/submit-flag", controller.SubmitFlag)
	sessions.POST("/:id/abandon", controller.AbandonSession)

	return r
}

// --- StartScenario tests ---

func TestStartScenario_Success(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create a scenario with steps
	scenario := models.Scenario{
		Name:         "start-ctrl-test",
		Title:        "Start Controller Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "Step 1",
		TextContent: "Do something",
	}
	require.NoError(t, db.Create(&step).Error)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenarios/"+scenario.ID.String()+"/start", nil)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotEmpty(t, response["ID"])
	assert.Equal(t, "active", response["status"])
}

func TestStartScenario_InvalidID(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenarios/not-a-uuid/start", nil)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestStartScenario_NotFound(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	fakeID := uuid.New()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenarios/"+fakeID.String()+"/start", nil)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- GetCurrentStep tests ---

func TestGetCurrentStep_Success(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	scenario := models.Scenario{
		Name:         "get-step-ctrl",
		Title:        "Get Step Controller",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:  scenario.ID,
		Order:       0,
		Title:       "First Step",
		TextContent: "Do this",
		HintContent: "Hint here",
		HasFlag:     true,
	}
	require.NoError(t, db.Create(&step).Error)

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "test-user-123",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	progress := models.ScenarioStepProgress{
		SessionID: session.ID,
		StepOrder: 0,
		Status:    "active",
	}
	require.NoError(t, db.Create(&progress).Error)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/"+session.ID.String()+"/current-step", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "First Step", response["title"])
	assert.Equal(t, "Do this", response["text"])
	assert.Equal(t, "active", response["status"])
	assert.Equal(t, true, response["has_flag"])
}

func TestGetCurrentStep_InvalidID(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/bad-uuid/current-step", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- VerifyStep tests ---

func TestVerifyStep_NoTerminal(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	scenario := models.Scenario{
		Name:         "verify-no-term",
		Title:        "Verify No Terminal",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID:   scenario.ID,
		Order:        0,
		Title:        "Step 1",
		VerifyScript: "#!/bin/bash\ntrue",
	}
	require.NoError(t, db.Create(&step).Error)

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "test-user-123",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
		// No TerminalSessionID
	}
	require.NoError(t, db.Create(&session).Error)

	sp := models.ScenarioStepProgress{SessionID: session.ID, StepOrder: 0, Status: "active"}
	require.NoError(t, db.Create(&sp).Error)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/"+session.ID.String()+"/verify", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- SubmitFlag tests ---

func TestSubmitFlag_Success(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	scenario := models.Scenario{
		Name:         "flag-ctrl-test",
		Title:        "Flag Controller Test",
		InstanceType: "ubuntu:22.04",
		FlagsEnabled: true,
		FlagSecret:   "secret",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	step := models.ScenarioStep{
		ScenarioID: scenario.ID,
		Order:      0,
		Title:      "Step 1",
		HasFlag:    true,
	}
	require.NoError(t, db.Create(&step).Error)

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "test-user-123",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	flag := models.ScenarioFlag{
		SessionID:    session.ID,
		StepOrder:    0,
		ExpectedFlag: "FLAG{abcdef1234567890}",
	}
	require.NoError(t, db.Create(&flag).Error)

	body, _ := json.Marshal(map[string]string{"flag": "FLAG{abcdef1234567890}"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/"+session.ID.String()+"/submit-flag", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, true, response["correct"])
}

func TestSubmitFlag_MissingBody(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	session := models.ScenarioSession{
		ScenarioID:  uuid.New(),
		UserID:      "test-user-123",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
	}
	// We just need any session ID for the URL; the body validation should fail first
	require.NoError(t, db.Create(&session).Error)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/"+session.ID.String()+"/submit-flag", nil)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- AbandonSession tests ---

func TestAbandonSession_Success(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	scenario := models.Scenario{
		Name:         "abandon-ctrl",
		Title:        "Abandon Controller",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "test-user-123",
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/"+session.ID.String()+"/abandon", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify session is abandoned in DB
	var updated models.ScenarioSession
	db.First(&updated, "id = ?", session.ID)
	assert.Equal(t, "abandoned", updated.Status)
}

func TestAbandonSession_NotFound(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	fakeID := uuid.New()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/"+fakeID.String()+"/abandon", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- ImportScenario tests ---

func TestImportScenario_NotImplemented(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	body, _ := json.Marshal(map[string]string{
		"git_repository": "https://example.com/repo.git",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenarios/import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
}
