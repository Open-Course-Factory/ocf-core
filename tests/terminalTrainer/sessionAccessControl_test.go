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

	// Should detect expiration
	assert.NoError(t, err)
	assert.False(t, isValid)
	assert.Equal(t, "expired", reason)

	// Verify status was updated in database
	var updatedTerminal models.Terminal
	err = db.Where("session_id = ?", terminal.SessionID).First(&updatedTerminal).Error
	assert.NoError(t, err)
	assert.Equal(t, "expired", updatedTerminal.Status)
}

// TestValidateSessionAccess_StoppedSession tests that stopped sessions are rejected
func TestValidateSessionAccess_StoppedSession(t *testing.T) {
	db := setupTestDB(t)
	service := services.NewTerminalTrainerService(db)

	// Create a stopped terminal
	terminal, err := createTestTerminal(db, "test-user", "stopped", time.Now().Add(1*time.Hour))
	assert.NoError(t, err)

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

	// Create a stopped terminal
	terminal, err := createTestTerminal(db, "test-user-123", "stopped", time.Now().Add(1*time.Hour))
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

// TestGetAccessStatus_ActiveSession tests the access status endpoint with active session
func TestGetAccessStatus_ActiveSession(t *testing.T) {
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
