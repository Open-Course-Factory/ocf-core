package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
)

// TestValidateSessionAccess_ExpiredSession tests that expired sessions are detected
func TestValidateSessionAccess_ExpiredSession(t *testing.T) {
	db := setupTestDB(t)
	service := services.NewTerminalTrainerService(db)

	// Create an expired terminal (expires 1 hour ago)
	terminal, err := createTestTerminal(db, "test-user", "active", time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)

	// Validate session access
	isValid, reason, err := service.ValidateSessionAccess(terminal.SessionID, false)

	// Should detect expiration. The legacy Status side-write is intentionally
	// gone — Status is being deprecated in favour of State (populated by
	// SyncUserSessions from tt-backend's authoritative session.state).
	assert.NoError(t, err)
	assert.False(t, isValid)
	assert.Equal(t, "expired", reason)
}

// TestValidateSessionAccess_StoppedSession tests that stopped sessions are rejected
func TestValidateSessionAccess_StoppedSession(t *testing.T) {
	db := setupTestDB(t)
	service := services.NewTerminalTrainerService(db)

	// Create a stopped terminal. State is the canonical SSOT.
	userKey, err := createTestUserKey(db, "test-user")
	assert.NoError(t, err)
	terminal := &models.Terminal{
		SessionID:         "test-stopped-session-basic",
		UserID:            "test-user",
		Name:              "Test Terminal",
		Status:            "stopped",
		State:             "stopped",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	assert.NoError(t, db.Create(terminal).Error)

	// Validate session access
	isValid, reason, err := service.ValidateSessionAccess(terminal.SessionID, false)

	// Should reject stopped session
	assert.NoError(t, err)
	assert.False(t, isValid)
	assert.Equal(t, "stopped", reason)
}

// TestValidateSessionAccess_ActiveSession tests that active sessions are allowed
func TestValidateSessionAccess_ActiveSession(t *testing.T) {
	db := setupTestDB(t)
	service := services.NewTerminalTrainerService(db)

	// Create an active terminal (expires in 1 hour)
	terminal, err := createTestTerminal(db, "test-user", "active", time.Now().Add(1*time.Hour))
	assert.NoError(t, err)

	// Validate session access
	isValid, reason, err := service.ValidateSessionAccess(terminal.SessionID, false)

	// Should allow access
	assert.NoError(t, err)
	assert.True(t, isValid)
	assert.Equal(t, "active", reason)
}

// TestGetAccessStatus_StoppedSession tests the access status endpoint with stopped session
func TestGetAccessStatus_StoppedSession(t *testing.T) {
	db := setupTestDB(t)
	controller := terminalController.NewTerminalController(db)

	// Create a stopped terminal. State is the canonical SSOT.
	userKey, err := createTestUserKey(db, "test-user-123")
	assert.NoError(t, err)
	terminal := &models.Terminal{
		SessionID:         "test-stopped-access-status",
		UserID:            "test-user-123",
		Name:              "Test Terminal",
		Status:            "stopped",
		State:             "stopped",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	assert.NoError(t, db.Create(terminal).Error)

	// Setup gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware
	router.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-123")
		c.Set("userRoles", []string{"user"})
		c.Next()
	})

	router.GET("/terminals/:id/access-status", controller.GetAccessStatus)

	// Make request (use SessionID which is what the endpoint expects)
	req := httptest.NewRequest("GET", "/terminals/"+terminal.SessionID+"/access-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 200 OK with accessibility info
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, unmarshalErr)

	// Verify response
	assert.True(t, response["has_permission"].(bool), "User should have permission (owner)")
	assert.False(t, response["can_access_console"].(bool), "Should not be able to access console")
	assert.Equal(t, "stopped", response["session_status"].(string))
	assert.Equal(t, "stopped", response["denial_reason"].(string))
}

// TestGetAccessStatus_ExpiredSession tests the access status endpoint with expired session
func TestGetAccessStatus_ExpiredSession(t *testing.T) {
	db := setupTestDB(t)
	controller := terminalController.NewTerminalController(db)

	// Create an expired terminal
	terminal, err := createTestTerminal(db, "test-user-123", "active", time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)

	// Setup gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware
	router.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-123")
		c.Set("userRoles", []string{"user"})
		c.Next()
	})

	router.GET("/terminals/:id/access-status", controller.GetAccessStatus)

	// Make request (use SessionID which is what the endpoint expects)
	req := httptest.NewRequest("GET", "/terminals/"+terminal.SessionID+"/access-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 200 OK with accessibility info
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, unmarshalErr)

	// Verify response
	assert.True(t, response["has_permission"].(bool), "User should have permission (owner)")
	assert.False(t, response["can_access_console"].(bool), "Should not be able to access console")
	assert.Equal(t, "expired", response["session_status"].(string))
	assert.Equal(t, "expired", response["denial_reason"].(string))
}

// TestGetAccessStatus_ActiveSession tests the access status endpoint with active session.
// Requires a running Terminal Trainer backend (ValidateSessionAccess calls the API).
func TestGetAccessStatus_ActiveSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode (requires Terminal Trainer API)")
	}
	db := setupTestDB(t)
	controller := terminalController.NewTerminalController(db)

	// Create an active terminal
	terminal, err := createTestTerminal(db, "test-user-123", "active", time.Now().Add(1*time.Hour))
	assert.NoError(t, err)

	// Setup gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware
	router.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-123")
		c.Set("userRoles", []string{"user"})
		c.Next()
	})

	router.GET("/terminals/:id/access-status", controller.GetAccessStatus)

	// Make request (use SessionID which is what the endpoint expects)
	req := httptest.NewRequest("GET", "/terminals/"+terminal.SessionID+"/access-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 200 OK with accessibility info
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, unmarshalErr)

	// Verify response
	assert.True(t, response["has_permission"].(bool), "User should have permission (owner)")
	assert.True(t, response["can_access_console"].(bool), "Should be able to access console")
	assert.Equal(t, "active", response["session_status"].(string))
	assert.NotContains(t, response, "denial_reason", "Should not have denial reason for active session")
}

// TestGetAccessStatus_NoPermission tests the access status endpoint without permission
func TestGetAccessStatus_NoPermission(t *testing.T) {
	db := setupTestDB(t)
	controller := terminalController.NewTerminalController(db)

	// Create an active terminal owned by another user
	terminal, err := createTestTerminal(db, "owner-user", "active", time.Now().Add(1*time.Hour))
	assert.NoError(t, err)

	// Setup gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware (different user, not owner)
	router.Use(func(c *gin.Context) {
		c.Set("userId", "different-user")
		c.Set("userRoles", []string{"user"})
		c.Next()
	})

	router.GET("/terminals/:id/access-status", controller.GetAccessStatus)

	// Make request (use SessionID which is what the endpoint expects)
	req := httptest.NewRequest("GET", "/terminals/"+terminal.SessionID+"/access-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 200 OK with accessibility info
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, unmarshalErr)

	// Verify response
	assert.False(t, response["has_permission"].(bool), "User should not have permission")
	assert.False(t, response["can_access_console"].(bool), "Should not be able to access console")
	assert.Equal(t, "no_permission", response["denial_reason"].(string))
}

// TestValidateSessionAccess_StateStopped_ReturnsStoppedNotExpired reproduces
// the zombie-slot bug: a persistent session whose 60s expiry passed gets
// auto-stopped by tt-backend (sessions.state='stopped'). The sync mirrors
// State='stopped' into ocf-core, but the legacy Status field drifts to
// 'expired' once ExpiresAt is in the past. The frontend Resume button POSTs
// /terminals/:id/start, which goes through ValidateSessionAccess. Before the
// fix, this returns ("expired") because the function reads the legacy Status
// field. After the fix, it must return ("stopped") so the middleware's
// allowStopped branch lets the Start handler run.
func TestValidateSessionAccess_StateStopped_ReturnsStoppedNotExpired(t *testing.T) {
	db := setupTestDB(t)
	service := services.NewTerminalTrainerService(db)

	userKey, err := createTestUserKey(db, "test-user")
	assert.NoError(t, err)

	// Reproduce the exact bug shape: tt-backend auto-stopped the session
	// (State='stopped'), but ExpiresAt is in the past and the legacy Status
	// drifted to 'expired'. The canonical truth is State.
	terminal := &models.Terminal{
		SessionID:         "test-stopped-session",
		UserID:            "test-user",
		Name:              "Persistent",
		Status:            "expired", // drifted legacy field — what triggers the bug
		State:             "stopped", // canonical SSOT — auto-stop succeeded
		PersistenceMode:   "persistent",
		ExpiresAt:         time.Now().Add(-time.Hour), // past — would also be "expired" by Status path
		UserTerminalKeyID: userKey.ID,
	}
	assert.NoError(t, db.Create(terminal).Error)

	isValid, reason, err := service.ValidateSessionAccess(terminal.SessionID, false)

	assert.NoError(t, err)
	assert.False(t, isValid)
	assert.Equal(t, "stopped", reason,
		"persistent session with State='stopped' must report 'stopped' so the middleware's allowStopped branch lets Resume succeed; reading legacy Status returns 'expired' and 410-Gone's the user")
}

// TestValidateSessionAccess_StateEmpty_FallsBackToStatus pins the migration
// path: pre-State rows (empty State) keep behaving on the legacy Status
// field so old data doesn't suddenly break.
func TestValidateSessionAccess_StateEmpty_FallsBackToStatus(t *testing.T) {
	db := setupTestDB(t)
	service := services.NewTerminalTrainerService(db)

	userKey, err := createTestUserKey(db, "test-user")
	assert.NoError(t, err)

	// Legacy row without a State value. SQLite's column default is 'running'
	// for new inserts, so we have to explicitly write "" via Update to
	// simulate a row that pre-dates the State column being populated.
	terminal := &models.Terminal{
		SessionID:         "test-legacy-session",
		UserID:            "test-user",
		Name:              "Legacy",
		Status:            "active",
		ExpiresAt:         time.Now().Add(time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	assert.NoError(t, db.Create(terminal).Error)
	// Force State to empty via raw SQL to bypass GORM's default.
	assert.NoError(t, db.Model(&models.Terminal{}).
		Where("session_id = ?", terminal.SessionID).
		Update("state", "").Error)

	isValid, reason, err := service.ValidateSessionAccess(terminal.SessionID, false)

	assert.NoError(t, err)
	assert.True(t, isValid, "legacy row with State='' and Status='active' must still be accessible")
	assert.Equal(t, "active", reason)
}

// TestValidateSessionAccess_StateDeleted_ReturnsExpiredWireFormat preserves
// the wire string the frontend already maps to "Session ended": when the
// canonical State is 'deleted', report 'expired' to keep the middleware's
// existing 410-Gone branch and the FE message intact.
func TestValidateSessionAccess_StateDeleted_ReturnsExpiredWireFormat(t *testing.T) {
	db := setupTestDB(t)
	service := services.NewTerminalTrainerService(db)

	userKey, err := createTestUserKey(db, "test-user")
	assert.NoError(t, err)

	terminal := &models.Terminal{
		SessionID:         "test-deleted-session",
		UserID:            "test-user",
		Name:              "Deleted",
		Status:            "deleted",
		State:             "deleted",
		ExpiresAt:         time.Now().Add(time.Hour),
		UserTerminalKeyID: userKey.ID,
	}
	assert.NoError(t, db.Create(terminal).Error)

	isValid, reason, err := service.ValidateSessionAccess(terminal.SessionID, false)

	assert.NoError(t, err)
	assert.False(t, isValid)
	assert.Equal(t, "expired", reason,
		"State='deleted' maps to the 'expired' wire format so the FE keeps showing 'Session ended'")
}
