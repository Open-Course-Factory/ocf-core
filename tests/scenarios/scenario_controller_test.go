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

	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
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
	sessions.GET("/:id/info", controller.GetSessionInfo)
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
	sessions.GET("/:id/info", controller.GetSessionInfo)

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

func TestSeedScenario_WithOsType(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	payload := map[string]any{
		"title":         "OsType Test Scenario",
		"instance_type": "ubuntu:22.04",
		"os_type":       "deb",
		"steps": []map[string]any{
			{"title": "Step 1", "text_content": "Do something"},
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
	assert.Equal(t, "deb", response["os_type"])

	// Verify persisted in DB
	var scenario models.Scenario
	db.First(&scenario, "name = ?", "ostype-test-scenario")
	assert.Equal(t, "deb", scenario.OsType)
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

// --- GetSessionInfo tests ---

func TestGetSessionInfo_OwnerCanAccess(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	scenario := models.Scenario{
		Name: "info-test", Title: "Info Test", InstanceType: "ubuntu:22.04", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	terminalID := "info-terminal-123"
	session := models.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "test-user-123", // matches setupTestRouter userId
		TerminalSessionID: &terminalID,
		CurrentStep:       1,
		Status:            "active",
		StartedAt:         time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/"+session.ID.String()+"/info", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, session.ID.String(), response["id"])
	assert.Equal(t, scenario.ID.String(), response["scenario_id"])
	assert.Equal(t, "test-user-123", response["user_id"])
	assert.Equal(t, "info-terminal-123", response["terminal_session_id"])
	assert.Equal(t, float64(1), response["current_step"])
	assert.Equal(t, "active", response["status"])
}

func TestGetSessionInfo_NonOwnerGetsForbidden(t *testing.T) {
	db := setupTestDB(t)

	scenario := models.Scenario{
		Name: "info-idor", Title: "Info IDOR", InstanceType: "ubuntu:22.04", CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	session := models.ScenarioSession{
		ScenarioID:  scenario.ID,
		UserID:      "user-A", // different from the user making the request
		CurrentStep: 0,
		Status:      "active",
		StartedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&session).Error)

	// Create router with user-B (different from session owner)
	router := setupTestRouterWithUser(db, "user-B")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/"+session.ID.String()+"/info", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- Group-based access control tests for StartScenario ---

// setupTestRouterWithRoles creates a test router with a custom userId and role list.
func setupTestRouterWithRoles(db *gorm.DB, userID string, roles []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})

	controller := scenarioController.NewScenarioController(db)

	sessions := api.Group("/scenario-sessions")
	sessions.POST("/start", controller.StartScenario)

	return r
}

// TestStartScenario_NoAssignment_Returns403 verifies that a regular (non-admin) user
// cannot start a scenario that is NOT assigned to any of their groups.
func TestStartScenario_NoAssignment_Returns403(t *testing.T) {
	db := setupTestDB(t)

	// Create a scenario with at least one step
	scenario := models.Scenario{
		Name:         "no-assignment-test",
		Title:        "No Assignment Test",
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

	// Create a terminal owned by our test user
	terminal := terminalModels.Terminal{
		SessionID: "terminal-no-assign",
		UserID:    "regular-user-1",
		Status:    "active",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(&terminal).Error)

	// No group, no group member, no assignment — user is just a regular member
	router := setupTestRouterWithRoles(db, "regular-user-1", []string{"member"})

	body, _ := json.Marshal(map[string]string{
		"scenario_id":         scenario.ID.String(),
		"terminal_session_id": "terminal-no-assign",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// Should be 403 because the user has no group assignment for this scenario
	assert.Equal(t, http.StatusForbidden, w.Code, "expected 403 when user has no assignment for the scenario")
}

// TestStartScenario_WithGroupAssignment_Succeeds verifies that a regular user whose
// group HAS an active assignment for the scenario can start it successfully.
func TestStartScenario_WithGroupAssignment_Succeeds(t *testing.T) {
	db := setupTestDB(t)

	// Create a scenario with a step
	scenario := models.Scenario{
		Name:         "with-assignment-test",
		Title:        "With Assignment Test",
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

	// Create group, member, and active assignment
	group := groupModels.ClassGroup{
		Name:        "test-group-assigned",
		DisplayName: "Test Group Assigned",
		OwnerUserID: "creator-1",
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	member := groupModels.GroupMember{
		GroupID:  group.ID,
		UserID:   "assigned-user-1",
		Role:     groupModels.GroupMemberRoleMember,
		IsActive: true,
		JoinedAt: time.Now(),
	}
	require.NoError(t, db.Omit("Metadata").Create(&member).Error)

	assignment := models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &group.ID,
		Scope:       "group",
		IsActive:    true,
		CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&assignment).Error)

	// Create terminal owned by the user
	terminal := terminalModels.Terminal{
		SessionID: "terminal-assigned",
		UserID:    "assigned-user-1",
		Status:    "active",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(&terminal).Error)

	router := setupTestRouterWithRoles(db, "assigned-user-1", []string{"member"})

	body, _ := json.Marshal(map[string]string{
		"scenario_id":         scenario.ID.String(),
		"terminal_session_id": "terminal-assigned",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, "expected 201 when user's group has an active assignment")

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotEmpty(t, response["id"])
	assert.Equal(t, "active", response["status"])
}

// TestStartScenario_AdminBypassesAssignmentCheck verifies that a platform admin
// can start any scenario without needing a group assignment.
func TestStartScenario_AdminBypassesAssignmentCheck(t *testing.T) {
	db := setupTestDB(t)

	// Create a scenario with a step
	scenario := models.Scenario{
		Name:         "admin-bypass-test",
		Title:        "Admin Bypass Test",
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

	// Create terminal owned by the admin user
	terminal := terminalModels.Terminal{
		SessionID: "terminal-admin-bypass",
		UserID:    "admin-user-1",
		Status:    "active",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(&terminal).Error)

	// No group, no assignment — admin should bypass the check
	router := setupTestRouterWithRoles(db, "admin-user-1", []string{"admin"})

	body, _ := json.Marshal(map[string]string{
		"scenario_id":         scenario.ID.String(),
		"terminal_session_id": "terminal-admin-bypass",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, "expected 201 for admin even without assignment")

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotEmpty(t, response["id"])
	assert.Equal(t, "active", response["status"])
}

// TestStartScenario_ExpiredDeadline_Returns403 verifies that a user whose group
// assignment has a past deadline cannot start the scenario.
func TestStartScenario_ExpiredDeadline_Returns403(t *testing.T) {
	db := setupTestDB(t)

	// Create scenario with a step
	scenario := models.Scenario{
		Name:         "expired-deadline-test",
		Title:        "Expired Deadline Test",
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

	// Create group and member
	group := groupModels.ClassGroup{
		Name:        "test-group-expired",
		DisplayName: "Test Group Expired",
		OwnerUserID: "creator-1",
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	member := groupModels.GroupMember{
		GroupID:  group.ID,
		UserID:   "expired-user-1",
		Role:     groupModels.GroupMemberRoleMember,
		IsActive: true,
		JoinedAt: time.Now(),
	}
	require.NoError(t, db.Omit("Metadata").Create(&member).Error)

	// Create assignment with a past deadline
	pastDeadline := time.Now().Add(-24 * time.Hour)
	assignment := models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &group.ID,
		Scope:       "group",
		IsActive:    true,
		Deadline:    &pastDeadline,
		CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&assignment).Error)

	// Create terminal
	terminal := terminalModels.Terminal{
		SessionID: "terminal-expired",
		UserID:    "expired-user-1",
		Status:    "active",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(&terminal).Error)

	router := setupTestRouterWithRoles(db, "expired-user-1", []string{"member"})

	body, _ := json.Marshal(map[string]string{
		"scenario_id":         scenario.ID.String(),
		"terminal_session_id": "terminal-expired",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "expected 403 when assignment deadline has passed")
}

// TestStartScenario_InactiveAssignment_Returns403 verifies that a user whose group
// assignment has is_active=false cannot start the scenario.
func TestStartScenario_InactiveAssignment_Returns403(t *testing.T) {
	db := setupTestDB(t)

	// Create scenario with a step
	scenario := models.Scenario{
		Name:         "inactive-assignment-test",
		Title:        "Inactive Assignment Test",
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

	// Create group and member
	group := groupModels.ClassGroup{
		Name:        "test-group-inactive",
		DisplayName: "Test Group Inactive",
		OwnerUserID: "creator-1",
	}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	member := groupModels.GroupMember{
		GroupID:  group.ID,
		UserID:   "inactive-user-1",
		Role:     groupModels.GroupMemberRoleMember,
		IsActive: true,
		JoinedAt: time.Now(),
	}
	require.NoError(t, db.Omit("Metadata").Create(&member).Error)

	// Create assignment then set is_active=false
	// (GORM default:true treats false as zero value and overrides it)
	assignment := models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &group.ID,
		Scope:       "group",
		IsActive:    true,
		CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&assignment).Error)
	require.NoError(t, db.Model(&assignment).Update("is_active", false).Error)

	// Create terminal
	terminal := terminalModels.Terminal{
		SessionID: "terminal-inactive",
		UserID:    "inactive-user-1",
		Status:    "active",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(&terminal).Error)

	router := setupTestRouterWithRoles(db, "inactive-user-1", []string{"member"})

	body, _ := json.Marshal(map[string]string{
		"scenario_id":         scenario.ID.String(),
		"terminal_session_id": "terminal-inactive",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/scenario-sessions/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "expected 403 when assignment is inactive")
}

// --- Available Scenarios endpoint tests ---

// setupAvailableRouter creates a test router with the /scenario-sessions/available route
func setupAvailableRouter(db *gorm.DB, userID string, roles []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	controller := scenarioController.NewScenarioController(db)
	sessions := api.Group("/scenario-sessions")
	sessions.GET("/available", controller.GetAvailableScenarios)
	return r
}

// TestGetAvailableScenarios_ReturnsOnlyAssigned verifies that a regular user
// only sees scenarios assigned to their group.
func TestGetAvailableScenarios_ReturnsOnlyAssigned(t *testing.T) {
	db := setupTestDB(t)

	// Create 3 scenarios
	scenario1 := models.Scenario{Name: "avail-assigned", Title: "Assigned Scenario", InstanceType: "debian", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&scenario1).Error)
	scenario2 := models.Scenario{Name: "avail-other1", Title: "Other Scenario 1", InstanceType: "debian", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&scenario2).Error)
	scenario3 := models.Scenario{Name: "avail-other2", Title: "Other Scenario 2", InstanceType: "debian", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&scenario3).Error)

	// Create group and add user as member
	group := groupModels.ClassGroup{Name: "avail-group", DisplayName: "Available Group", OwnerUserID: "creator-1"}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	member := groupModels.GroupMember{
		GroupID:  group.ID,
		UserID:   "user-avail-1",
		Role:     groupModels.GroupMemberRoleMember,
		IsActive: true,
		JoinedAt: time.Now(),
	}
	require.NoError(t, db.Omit("Metadata").Create(&member).Error)

	// Assign only scenario1 to the group
	assignment := models.ScenarioAssignment{
		ScenarioID:  scenario1.ID,
		GroupID:     &group.ID,
		Scope:       "group",
		IsActive:    true,
		CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&assignment).Error)

	router := setupAvailableRouter(db, "user-avail-1", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/available", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var scenarios []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &scenarios)
	require.NoError(t, err)
	assert.Len(t, scenarios, 1, "should return only the 1 assigned scenario")
	assert.Equal(t, scenario1.ID.String(), scenarios[0]["id"])
}

// TestGetAvailableScenarios_AdminSeesAll verifies that an admin user
// sees all scenarios regardless of assignments.
func TestGetAvailableScenarios_AdminSeesAll(t *testing.T) {
	db := setupTestDB(t)

	// Create 3 scenarios with no assignments
	scenario1 := models.Scenario{Name: "admin-all-1", Title: "Admin All 1", InstanceType: "debian", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&scenario1).Error)
	scenario2 := models.Scenario{Name: "admin-all-2", Title: "Admin All 2", InstanceType: "debian", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&scenario2).Error)
	scenario3 := models.Scenario{Name: "admin-all-3", Title: "Admin All 3", InstanceType: "debian", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&scenario3).Error)

	router := setupAvailableRouter(db, "admin-user-all", []string{"admin"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/available", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var scenarios []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &scenarios)
	require.NoError(t, err)
	assert.Len(t, scenarios, 3, "admin should see all 3 scenarios")
}

// TestGetAvailableScenarios_EmptyWhenNoAssignments verifies that a user
// in a group with no scenario assignments gets an empty array.
func TestGetAvailableScenarios_EmptyWhenNoAssignments(t *testing.T) {
	db := setupTestDB(t)

	// Create a scenario (but don't assign it)
	scenario := models.Scenario{Name: "empty-test", Title: "Empty Test", InstanceType: "debian", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&scenario).Error)

	// Create group and add user as member
	group := groupModels.ClassGroup{Name: "empty-group", DisplayName: "Empty Group", OwnerUserID: "creator-1"}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	member := groupModels.GroupMember{
		GroupID:  group.ID,
		UserID:   "user-empty-1",
		Role:     groupModels.GroupMemberRoleMember,
		IsActive: true,
		JoinedAt: time.Now(),
	}
	require.NoError(t, db.Omit("Metadata").Create(&member).Error)

	router := setupAvailableRouter(db, "user-empty-1", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/available", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var scenarios []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &scenarios)
	require.NoError(t, err)
	assert.Len(t, scenarios, 0, "should return empty array when no assignments exist")
}

// TestGetAvailableScenarios_ExcludesExpiredAndInactive verifies that only active
// assignments with no past deadline are returned.
func TestGetAvailableScenarios_ExcludesExpiredAndInactive(t *testing.T) {
	db := setupTestDB(t)

	// Create 3 scenarios
	activeScenario := models.Scenario{Name: "filter-active", Title: "Active Scenario", InstanceType: "debian", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&activeScenario).Error)
	expiredScenario := models.Scenario{Name: "filter-expired", Title: "Expired Scenario", InstanceType: "debian", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&expiredScenario).Error)
	inactiveScenario := models.Scenario{Name: "filter-inactive", Title: "Inactive Scenario", InstanceType: "debian", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&inactiveScenario).Error)

	// Create group and add user as member
	group := groupModels.ClassGroup{Name: "filter-group", DisplayName: "Filter Group", OwnerUserID: "creator-1"}
	require.NoError(t, db.Omit("Metadata").Create(&group).Error)

	member := groupModels.GroupMember{
		GroupID:  group.ID,
		UserID:   "user-filter-1",
		Role:     groupModels.GroupMemberRoleMember,
		IsActive: true,
		JoinedAt: time.Now(),
	}
	require.NoError(t, db.Omit("Metadata").Create(&member).Error)

	// Active assignment (no deadline, is_active=true)
	activeAssignment := models.ScenarioAssignment{
		ScenarioID:  activeScenario.ID,
		GroupID:     &group.ID,
		Scope:       "group",
		IsActive:    true,
		CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&activeAssignment).Error)

	// Expired assignment (past deadline)
	pastDeadline := time.Now().Add(-24 * time.Hour)
	expiredAssignment := models.ScenarioAssignment{
		ScenarioID:  expiredScenario.ID,
		GroupID:     &group.ID,
		Scope:       "group",
		IsActive:    true,
		Deadline:    &pastDeadline,
		CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&expiredAssignment).Error)

	// Inactive assignment (is_active=false)
	inactiveAssignment := models.ScenarioAssignment{
		ScenarioID:  inactiveScenario.ID,
		GroupID:     &group.ID,
		Scope:       "group",
		IsActive:    true,
		CreatedByID: "creator-1",
	}
	require.NoError(t, db.Create(&inactiveAssignment).Error)
	require.NoError(t, db.Model(&inactiveAssignment).Update("is_active", false).Error)

	router := setupAvailableRouter(db, "user-filter-1", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/available", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var scenarios []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &scenarios)
	require.NoError(t, err)
	assert.Len(t, scenarios, 1, "should return only the active, non-expired scenario")
	assert.Equal(t, activeScenario.ID.String(), scenarios[0]["id"])
}

// TestGetAvailableScenarios_IncludesOrgAssignment verifies that a user
// sees scenarios assigned to their organization even if not assigned to their group.
func TestGetAvailableScenarios_IncludesOrgAssignment(t *testing.T) {
	db := setupTestDB(t)

	// Create a scenario
	scenario := models.Scenario{Name: "org-avail", Title: "Org Available", InstanceType: "debian", CreatedByID: "creator-1"}
	require.NoError(t, db.Create(&scenario).Error)

	// Create organization and add user as org member
	org := orgModels.Organization{Name: "test-org-avail", DisplayName: "Test Org Avail", OwnerUserID: "creator-1"}
	require.NoError(t, db.Omit("Metadata").Create(&org).Error)

	orgMember := orgModels.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         "user-org-1",
		Role:           orgModels.OrgRoleMember,
		IsActive:       true,
		JoinedAt:       time.Now(),
	}
	require.NoError(t, db.Omit("Metadata").Create(&orgMember).Error)

	// Assign scenario to the org (scope="org"), NOT to any group
	assignment := models.ScenarioAssignment{
		ScenarioID:     scenario.ID,
		OrganizationID: &org.ID,
		Scope:          "org",
		IsActive:       true,
		CreatedByID:    "creator-1",
	}
	require.NoError(t, db.Create(&assignment).Error)

	router := setupAvailableRouter(db, "user-org-1", []string{"member"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/scenario-sessions/available", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var scenarios []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &scenarios)
	require.NoError(t, err)
	assert.Len(t, scenarios, 1, "should return the org-assigned scenario")
	assert.Equal(t, scenario.ID.String(), scenarios[0]["id"])
}
