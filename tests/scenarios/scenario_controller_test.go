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
	terminalModels "soli/formations/src/terminalTrainer/models"

	"gorm.io/gorm"
)

func setupTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	// Set userId and roles directly to bypass auth middleware in tests
	api.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-123")
		c.Set("userRoles", []string{"admin"})
		c.Next()
	})

	controller := scenarioController.NewScenarioController(db)

	// Scenario management routes
	scenarios := api.Group("/scenarios")
	scenarios.POST("/import", controller.ImportScenario)
	scenarios.POST("/seed", controller.SeedScenario)

	// Session routes
	sessions := api.Group("/scenario-sessions")
	sessions.POST("/start", controller.StartScenario)
	sessions.GET("/by-terminal/:terminalId", controller.GetSessionByTerminal)
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

	// Create terminal owned by the test user (required for ownership check)
	terminal := terminalModels.Terminal{
		SessionID: "test-terminal-ctrl",
		UserID:    "test-user-123",
		Status:    "active",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(&terminal).Error)

	body, _ := json.Marshal(map[string]string{
		"scenario_id":        scenario.ID.String(),
		"terminal_session_id": "test-terminal-ctrl",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotEmpty(t, response["id"])
	assert.Equal(t, "active", response["status"])
	assert.Equal(t, "test-terminal-ctrl", response["terminal_session_id"])
}

func TestStartScenario_InvalidID(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	body, _ := json.Marshal(map[string]string{
		"scenario_id":        "not-a-uuid",
		"terminal_session_id": "test-terminal",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestStartScenario_NotFound(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create terminal owned by the test user (required for ownership check)
	terminal := terminalModels.Terminal{
		SessionID: "test-terminal-nf",
		UserID:    "test-user-123",
		Status:    "active",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(&terminal).Error)

	fakeID := uuid.New()
	body, _ := json.Marshal(map[string]string{
		"scenario_id":        fakeID.String(),
		"terminal_session_id": "test-terminal-nf",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/start", bytes.NewBuffer(body))
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

	assert.Equal(t, http.StatusNotFound, w.Code)
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

// --- GetSessionByTerminal tests ---

func TestGetSessionByTerminal_Success(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	scenario := models.Scenario{
		Name:         "terminal-lookup",
		Title:        "Terminal Lookup Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	terminalID := "terminal-lookup-abc"
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "test-user-123",
		TerminalSessionID: &terminalID,
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/by-terminal/"+terminalID, nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotEmpty(t, response["id"])
	assert.Equal(t, terminalID, response["terminal_session_id"])
	assert.Equal(t, "active", response["status"])
}

func TestGetSessionByTerminal_NotFound(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/by-terminal/unknown-terminal", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- SeedScenario tests ---

func TestSeedScenario_Success(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	payload := map[string]any{
		"title":          "My Seed Scenario",
		"description":    "A test scenario created via seed",
		"difficulty":     "beginner",
		"estimated_time": "15m",
		"instance_type":  "ubuntu:22.04",
		"flags_enabled":  true,
		"gsh_enabled":    false,
		"crash_traps":    true,
		"intro_text":     "# Welcome",
		"finish_text":    "# Done",
		"steps": []map[string]any{
			{
				"title":            "Step 1",
				"text_content":     "Do something",
				"hint_content":     "Try this",
				"verify_script":    "#!/bin/bash\ntrue",
				"background_script": "",
				"foreground_script": "",
				"has_flag":         true,
			},
			{
				"title":        "Step 2",
				"text_content": "Do another thing",
				"verify_script": "#!/bin/bash\ntest -f /tmp/file",
				"has_flag":     false,
			},
		},
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenarios/seed", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "My Seed Scenario", response["title"])
	assert.Equal(t, "my-seed-scenario", response["name"])
	assert.Equal(t, "seed", response["source_type"])
	assert.Equal(t, "beginner", response["difficulty"])
	assert.Equal(t, "ubuntu:22.04", response["instance_type"])
	assert.Equal(t, true, response["flags_enabled"])
	assert.Equal(t, true, response["crash_traps"])
	assert.Equal(t, "# Welcome", response["intro_text"])
	assert.Equal(t, "# Done", response["finish_text"])

	// Verify steps
	stepsRaw, ok := response["steps"].([]any)
	require.True(t, ok)
	assert.Len(t, stepsRaw, 2)

	step0 := stepsRaw[0].(map[string]any)
	assert.Equal(t, "Step 1", step0["title"])
	assert.Equal(t, "Do something", step0["text_content"])
	assert.Equal(t, float64(0), step0["order"])
	assert.Equal(t, true, step0["has_flag"])

	step1 := stepsRaw[1].(map[string]any)
	assert.Equal(t, "Step 2", step1["title"])
	assert.Equal(t, float64(1), step1["order"])
	assert.Equal(t, false, step1["has_flag"])

	// Verify persisted in DB
	var count int64
	db.Model(&models.Scenario{}).Count(&count)
	assert.Equal(t, int64(1), count)

	var stepCount int64
	db.Model(&models.ScenarioStep{}).Count(&stepCount)
	assert.Equal(t, int64(2), stepCount)
}

func TestSeedScenario_MissingTitle(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	payload := map[string]any{
		"steps": []map[string]any{
			{"title": "Step 1", "text_content": "Do something"},
		},
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenarios/seed", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// setupTestRouterWithUser creates a test router with a custom userId
func setupTestRouterWithUser(db *gorm.DB, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Next()
	})

	controller := scenarioController.NewScenarioController(db)

	sessions := api.Group("/scenario-sessions")
	sessions.GET("/by-terminal/:terminalId", controller.GetSessionByTerminal)

	return r
}

// --- IDOR tests ---

func TestGetSessionByTerminal_IDOR_Returns403(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name:         "idor-test",
		Title:        "IDOR Test",
		InstanceType: "ubuntu:22.04",
		CreatedByID:  "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	terminalID := "term-123"
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "user-A",
		TerminalSessionID: &terminalID,
		CurrentStep:       0,
		Status:            "active",
		StartedAt:         time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// Create router with user-B (different from session owner user-A)
	router := setupTestRouterWithUser(db, "user-B")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/by-terminal/"+terminalID, nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSeedScenario_NoSteps(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	payload := map[string]any{
		"title": "Empty Steps Scenario",
		"steps": []map[string]any{},
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenarios/seed", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
