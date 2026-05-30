// tests/terminalTrainer/getUserSessionsContract_test.go
//
// Contract test pinning the wire shape of GET /terminals/user-sessions.
// Status is being removed as a parallel SSOT; the response body must expose
// only `state`, not the legacy `status` field. This test would have caught
// the API surface regression where a terminal with State="stopped" still
// leaked the old Status value to the FE (which was the root cause of the
// zombie-resume / drifted-banner bugs).
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// TestGetUserSessions_ResponseExposesStateNotStatus pins the canonical wire
// contract: the response carries `state` (the SSOT) and does NOT carry the
// legacy `status` key on any terminal. Stripping `Status` from TerminalOutput
// is what removes the drifted parallel field.
func TestGetUserSessions_ResponseExposesStateNotStatus(t *testing.T) {
	// Stand up a fake tt-backend that mirrors the local stopped session so
	// the pre-list sync inside GetUserSessions does not reconcile it away.
	// Without this, the SSOT reconciliation rule (tt-backend missing → mark
	// deleted) flips state from "stopped" to "deleted" and the test would
	// be asserting on a row the test infrastructure itself just rewrote.
	sessionID := "contract-session-stopped"
	srv := sessionListContainingTTServer(t, sessionID, "stopped")
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	controller := terminalController.NewTerminalController(db)

	userKey, err := createTestUserKey(db, "contract-user")
	require.NoError(t, err)

	terminal := &models.Terminal{
		SessionID:         sessionID,
		UserID:            "contract-user",
		Name:              "Contract Stopped",
		State:             models.StateStopped,
		PersistenceMode:   "persistent",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(terminal).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "contract-user")
		c.Set("userRoles", []string{"user"})
		c.Next()
	})
	router.GET("/terminals/user-sessions", controller.GetUserSessions)

	req := httptest.NewRequest("GET", "/terminals/user-sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 1, "expected exactly one session in the response")

	row := rows[0]
	assert.Equal(t, "stopped", row["state"], "state must reflect the SSOT value the DB stored")

	// The legacy parallel field must NOT appear on the wire.
	_, hasStatus := row["status"]
	assert.False(t, hasStatus,
		"GET /terminals/user-sessions must not expose the legacy `status` key — "+
			"State is the SSOT; the FE has migrated to read state. Leaking status "+
			"is what caused the drifted Resume / banner regressions.")
}
