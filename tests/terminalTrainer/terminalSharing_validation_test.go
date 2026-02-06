package terminalTrainer_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestShareTerminal_EmptyRecipient tests that sharing without a recipient is rejected
func TestShareTerminal_EmptyRecipient(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	assert.NoError(t, err)

	// Create controller
	controller := terminalController.NewTerminalController(db)

	// Setup gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware
	router.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-123")
		c.Next()
	})

	router.POST("/terminals/:id/share", controller.ShareTerminal)

	t.Run("Empty sharedWithUserID should fail", func(t *testing.T) {
		// Create request with empty shared_with_user_id
		requestBody := dto.ShareTerminalRequest{
			SharedWithUserID: nil, // Not provided
			AccessLevel:      models.AccessLevelRead,
		}

		bodyBytes, _ := json.Marshal(requestBody)
		req := httptest.NewRequest("POST", "/terminals/test-terminal-id/share", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should return 400 Bad Request
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["error_message"], "Must specify either shared_with_user_id or shared_with_group_id")
	})

	t.Run("Empty string sharedWithUserID should fail", func(t *testing.T) {
		emptyString := ""
		requestBody := dto.ShareTerminalRequest{
			SharedWithUserID: &emptyString, // Empty string
			AccessLevel:      models.AccessLevelRead,
		}

		bodyBytes, _ := json.Marshal(requestBody)
		req := httptest.NewRequest("POST", "/terminals/test-terminal-id/share", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should return 400 Bad Request
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["error_message"], "Must specify either shared_with_user_id or shared_with_group_id")
	})
}
