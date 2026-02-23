package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	terminalController "soli/formations/src/terminalTrainer/routes"
)

// makeDeleteHistoryRequest creates a gin router and sends a DELETE request for session history
func makeDeleteHistoryRequest(t *testing.T, db *gorm.DB, sessionID string, userID string, userRoles []string) *httptest.ResponseRecorder {
	controller := terminalController.NewTerminalController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", userRoles)
		c.Next()
	})

	router.DELETE("/terminals/:id/history", controller.DeleteSessionHistory)

	req := httptest.NewRequest("DELETE", "/terminals/"+sessionID+"/history", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	return w
}

// makeDeleteAllHistoryRequest creates a gin router and sends a DELETE request for all user history
func makeDeleteAllHistoryRequest(t *testing.T, db *gorm.DB, userID string, userRoles []string) *httptest.ResponseRecorder {
	controller := terminalController.NewTerminalController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", userRoles)
		c.Next()
	})

	router.DELETE("/terminals/my-history", controller.DeleteAllUserHistory)

	req := httptest.NewRequest("DELETE", "/terminals/my-history", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	return w
}

// --- GetSessionHistory controller tests ---

// TestGetSessionHistory_NonexistentSession_Returns404 verifies that requesting
// history for a session that does not exist returns 404.
func TestGetSessionHistory_NonexistentSession_Returns404(t *testing.T) {
	db := setupTestDB(t)

	w := makeHistoryRequest(t, db, "nonexistent-session-id", "user1", []string{"user"})

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Session not found", response["error_message"])
}

// TestGetSessionHistory_Unauthorized_Returns403 verifies that a non-owner,
// non-admin user gets 403 when requesting another user's session history.
func TestGetSessionHistory_Unauthorized_Returns403(t *testing.T) {
	db := setupTestDB(t)

	// Create a terminal owned by "owner-user"
	terminal, err := createTestTerminal(db, "owner-user", "stopped", nil)
	require.NoError(t, err)

	// A different user (not owner, not admin) requests history
	w := makeHistoryRequest(t, db, terminal.SessionID, "other-user", []string{"user"})

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error_message"], "Only session owner or admin")
}

// TestGetSessionHistory_OwnerPassesAccessCheck verifies that the session owner
// passes the access check. The request will fail at the service level (no TT
// backend configured), but it must NOT return 403 or 404.
func TestGetSessionHistory_OwnerPassesAccessCheck(t *testing.T) {
	db := setupTestDB(t)

	terminal, err := createTestTerminal(db, "owner-user", "stopped", nil)
	require.NoError(t, err)

	w := makeHistoryRequest(t, db, terminal.SessionID, "owner-user", []string{"user"})

	// Should NOT be 403 (access denied) or 404 (not found)
	// Will be 500 because there's no real TT backend to proxy to, which is fine
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Owner should pass the access check")
	assert.NotEqual(t, http.StatusNotFound, w.Code,
		"Session should be found")
}

// TestGetSessionHistory_QueryParams verifies that query parameters (since, format,
// limit, offset) are parsed by the controller without errors.
func TestGetSessionHistory_QueryParams(t *testing.T) {
	db := setupTestDB(t)

	terminal, err := createTestTerminal(db, "owner-user", "stopped", nil)
	require.NoError(t, err)

	controller := terminalController.NewTerminalController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "owner-user")
		c.Set("userRoles", []string{"user"})
		c.Next()
	})
	router.GET("/terminals/:id/history", controller.GetSessionHistory)

	// Request with all query params
	req := httptest.NewRequest("GET", "/terminals/"+terminal.SessionID+"/history?since=1000&format=csv&limit=50&offset=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should NOT be 403 or 404 - params should be parsed without error
	assert.NotEqual(t, http.StatusForbidden, w.Code)
	assert.NotEqual(t, http.StatusNotFound, w.Code)
	assert.NotEqual(t, http.StatusBadRequest, w.Code,
		"Valid query parameters should not cause a 400 error")
}

// --- DeleteSessionHistory controller tests ---

// TestDeleteSessionHistory_NonexistentSession_Returns404 verifies that deleting
// history for a nonexistent session returns 404.
func TestDeleteSessionHistory_NonexistentSession_Returns404(t *testing.T) {
	db := setupTestDB(t)

	w := makeDeleteHistoryRequest(t, db, "nonexistent-session-id", "user1", []string{"user"})

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Session not found", response["error_message"])
}

// TestDeleteSessionHistory_Unauthorized_Returns403 verifies that a non-owner,
// non-admin user gets 403 when trying to delete another user's session history.
func TestDeleteSessionHistory_Unauthorized_Returns403(t *testing.T) {
	db := setupTestDB(t)

	terminal, err := createTestTerminal(db, "owner-user", "stopped", nil)
	require.NoError(t, err)

	w := makeDeleteHistoryRequest(t, db, terminal.SessionID, "other-user", []string{"user"})

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error_message"], "Only session owner or admin")
}

// TestDeleteSessionHistory_OwnerPassesAccessCheck verifies that the session owner
// passes the access check for deleting history.
func TestDeleteSessionHistory_OwnerPassesAccessCheck(t *testing.T) {
	db := setupTestDB(t)

	terminal, err := createTestTerminal(db, "owner-user", "stopped", nil)
	require.NoError(t, err)

	w := makeDeleteHistoryRequest(t, db, terminal.SessionID, "owner-user", []string{"user"})

	// Should NOT be 403 or 404
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Owner should pass the access check")
	assert.NotEqual(t, http.StatusNotFound, w.Code,
		"Session should be found")
}

// --- DeleteAllUserHistory controller tests ---

// TestDeleteAllUserHistory_NoApiKey_Returns500 verifies that when a user has no
// terminal API key, the endpoint returns 500.
func TestDeleteAllUserHistory_NoApiKey_Returns500(t *testing.T) {
	db := setupTestDB(t)

	// User "no-key-user" has no terminal API key
	w := makeDeleteAllHistoryRequest(t, db, "no-key-user", []string{"user"})

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Failed to get user API key", response["error_message"])
}

// TestDeleteAllUserHistory_WithApiKey_PassesKeyLookup verifies that when a user
// has an API key, the controller successfully retrieves it and proceeds to the
// service call (which will fail without a real TT backend, but must NOT fail at
// the key lookup stage).
func TestDeleteAllUserHistory_WithApiKey_PassesKeyLookup(t *testing.T) {
	db := setupTestDB(t)

	// Create a user with an API key
	_, err := createTestUserKey(db, "keyed-user")
	require.NoError(t, err)

	w := makeDeleteAllHistoryRequest(t, db, "keyed-user", []string{"user"})

	// Should NOT return the "Failed to get user API key" error
	assert.NotEqual(t, http.StatusNotFound, w.Code)

	if w.Code == http.StatusInternalServerError {
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEqual(t, "Failed to get user API key", response["error_message"],
			"Should have passed the API key lookup stage")
	}
}
