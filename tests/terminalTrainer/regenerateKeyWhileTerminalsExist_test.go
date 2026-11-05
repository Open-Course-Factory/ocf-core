// tests/terminalTrainer/regenerateKeyWhileTerminalsExist_test.go
//
// tt-backend only serves a session to the key that created it, so revoking a key
// cuts its owner off from every terminal still running or paused under it.
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

func seedTerminalOfKeyOwner(t *testing.T, db *gorm.DB, userID string, state models.TerminalState, expiresAt time.Time) {
	t.Helper()
	var key models.UserTerminalKey
	require.NoError(t, db.Where("user_id = ?", userID).First(&key).Error)
	require.NoError(t, db.Create(&models.Terminal{
		SessionID:         "session-" + uuid.New().String(),
		UserID:            userID,
		UserTerminalKeyID: key.ID,
		State:             state,
		ExpiresAt:         expiresAt,
	}).Error)
}

func regenerateKey(db *gorm.DB, userID string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/user-terminal-keys/regenerate", nil)
	ctx.Set("userId", userID)
	terminalController.NewUserTerminalKeyController(db).RegenerateKey(ctx)
	return recorder
}

func TestRegenerateKey_WithALiveTerminal_RefusesAndKeepsTheKey(t *testing.T) {
	future := time.Now().Add(time.Hour)
	for _, state := range []models.TerminalState{models.StateRunning, models.StateStopped} {
		t.Run(string(state), func(t *testing.T) {
			srv := newKeyAdminTTServer(t, 42, false)
			configureTTServer(t, srv.URL)
			db := freshTestDB(t)
			userID := "key-regen-" + uuid.New().String()
			seedProvisionedKey(t, db, userID, 42)
			seedTerminalOfKeyOwner(t, db, userID, state, future)

			recorder := regenerateKey(db, userID)

			assert.Equal(t, http.StatusConflict, recorder.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			assert.Equal(t, "terminals_in_use", body["reason"], "the front translates its message from this")
			assert.Empty(t, srv.recordedUpdates(), "tt-backend keeps the key active")
			assert.True(t, localKeyIsActive(t, db, userID))
		})
	}
}

func TestRegenerateKey_WithOnlyFinishedTerminals_Regenerates(t *testing.T) {
	future, past := time.Now().Add(time.Hour), time.Now().Add(-time.Hour)
	srv := newKeyAdminTTServer(t, 42, false)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "key-regen-free-" + uuid.New().String()
	seedProvisionedKey(t, db, userID, 42)
	seedTerminalOfKeyOwner(t, db, userID, models.StateDeleted, future)
	seedTerminalOfKeyOwner(t, db, userID, models.StateRunning, past) // a ghost row from core#502 must not block anyone

	recorder := regenerateKey(db, userID)

	assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Len(t, srv.recordedUpdates(), 1, "the old key is deactivated on tt-backend")
}
