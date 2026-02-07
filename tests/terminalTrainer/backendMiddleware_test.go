package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soli/formations/src/terminalTrainer/models"
	terminalMiddleware "soli/formations/src/terminalTrainer/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMiddleware_NoBackend_Passes tests that terminals without a backend pass through middleware
func TestMiddleware_NoBackend_Passes(t *testing.T) {
	db := setupTestDB(t)

	// Create an active terminal without backend (backward compat)
	terminal, err := createTestTerminal(db, "test-user-mw", "active", time.Now().Add(1*time.Hour))
	require.NoError(t, err)

	tam := terminalMiddleware.NewTerminalAccessMiddleware(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware
	router.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-mw")
		c.Set("userRoles", []string{"user"})
		c.Next()
	})

	router.GET("/terminals/:id/test",
		tam.RequireTerminalAccess(models.AccessLevelRead),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "passed"})
		},
	)

	req := httptest.NewRequest("GET", "/terminals/"+terminal.SessionID+"/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Terminal with no backend should pass middleware (backward compat)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "passed", response["message"])
}

// TestMiddleware_StoppedSession_ReturnsForbidden tests that stopped sessions are rejected
func TestMiddleware_StoppedSession_ReturnsForbidden(t *testing.T) {
	db := setupTestDB(t)

	// Create a stopped terminal
	terminal, err := createTestTerminal(db, "test-user-mw2", "stopped", time.Now().Add(1*time.Hour))
	require.NoError(t, err)

	tam := terminalMiddleware.NewTerminalAccessMiddleware(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-mw2")
		c.Set("userRoles", []string{"user"})
		c.Next()
	})

	router.GET("/terminals/:id/test",
		tam.RequireTerminalAccess(models.AccessLevelRead),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "should not reach"})
		},
	)

	req := httptest.NewRequest("GET", "/terminals/"+terminal.SessionID+"/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Stopped sessions should be rejected
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestMiddleware_ExpiredSession_ReturnsGone tests that expired sessions return 410
func TestMiddleware_ExpiredSession_ReturnsGone(t *testing.T) {
	db := setupTestDB(t)

	// Create an expired terminal
	terminal, err := createTestTerminal(db, "test-user-mw3", "active", time.Now().Add(-1*time.Hour))
	require.NoError(t, err)

	tam := terminalMiddleware.NewTerminalAccessMiddleware(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-mw3")
		c.Set("userRoles", []string{"user"})
		c.Next()
	})

	router.GET("/terminals/:id/test",
		tam.RequireTerminalAccess(models.AccessLevelRead),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "should not reach"})
		},
	)

	req := httptest.NewRequest("GET", "/terminals/"+terminal.SessionID+"/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Expired sessions should return 410 Gone
	assert.Equal(t, http.StatusGone, w.Code)
}
