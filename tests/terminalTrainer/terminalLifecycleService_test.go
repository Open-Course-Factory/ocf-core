package terminalTrainer_tests

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// These tests exercise the session-lifecycle concern (extracted into
// terminalLifecycleService) through the public TerminalTrainerService facade,
// asserting the observable result: the persisted Terminal row and the
// ValidateSessionAccess reason. They complement terminalLifecycleProxy_test
// (which pins the proxy wiring + ownership) and lifecycleSyncBeforeValidate_test
// (which pins the sync-before-validate ordering).

// TestLifecycleService_StopSessionPersistent_TransitionsRowToStopped verifies
// that StopSession on a persistent session calls tt-backend's /stop and routes
// through markSessionStopped: the local row becomes StateStopped and the
// idle_until returned by tt-backend is mirrored onto the row (so the reserved
// capacity is held until sync confirms the reap).
func TestLifecycleService_StopSessionPersistent_TransitionsRowToStopped(t *testing.T) {
	ttServer, _ := startLifecycleTTServer(t)
	defer ttServer.Close()

	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	db := freshTestDB(t)
	userID := "stop-persistent-" + uuid.New().String()
	seedActiveSubscription(t, db, userID)

	terminal, err := createTestTerminal(db, userID, "active", time.Now().Add(time.Hour))
	require.NoError(t, err)
	terminal.PersistenceMode = "persistent"
	require.NoError(t, db.Save(terminal).Error)

	svc := services.NewTerminalTrainerService(db)
	require.NoError(t, svc.StopSession(terminal.SessionID))

	updated, err := svc.GetSessionInfo(terminal.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.StateStopped, updated.State,
		"persistent stop must leave the row in StateStopped")
	require.NotNil(t, updated.IdleUntil,
		"markSessionStopped must mirror the tt-backend idle_until onto the row")
	assert.True(t, updated.IdleUntil.After(time.Now()),
		"idle_until must be the future reap deadline returned by /stop")
}

// TestLifecycleService_ValidateSessionAccess_DeniesStoppedSession verifies that
// a stopped session is denied with the "stopped" reason without any API
// round-trip (checkAPI=false).
func TestLifecycleService_ValidateSessionAccess_DeniesStoppedSession(t *testing.T) {
	db := freshTestDB(t)
	userID := "validate-stopped-" + uuid.New().String()

	terminal, err := createTestTerminal(db, userID, "active", time.Now().Add(time.Hour))
	require.NoError(t, err)
	// Move the row to stopped directly so the validate path reads it.
	require.NoError(t, db.Model(&models.Terminal{}).
		Where("session_id = ?", terminal.SessionID).
		Update("state", models.StateStopped).Error)

	// No backend URL needed: checkAPI=false short-circuits before any API call.
	svc := services.NewTerminalTrainerService(db)
	ok, reason, err := svc.ValidateSessionAccess(terminal.SessionID, false)
	require.NoError(t, err)
	assert.False(t, ok, "a stopped session must not be accessible")
	assert.Equal(t, "stopped", reason)
}

// TestLifecycleService_StartSession_TransitionsRowToRunning verifies that
// StartSession on a previously stopped session calls tt-backend's /start and
// transitions the local row back to StateRunning, mirroring the recomputed
// expires_at and clearing idle_until.
func TestLifecycleService_StartSession_TransitionsRowToRunning(t *testing.T) {
	newExpiry := time.Now().Add(2 * time.Hour).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start") {
			w.Header().Set("Content-Type", "application/json")
			// tt-backend /start returns the recomputed expires_at (unix seconds).
			_, _ = w.Write([]byte(`{"expires_at":` + strconv.FormatInt(newExpiry, 10) + `}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("TERMINAL_TRAINER_URL", srv.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	db := freshTestDB(t)
	userID := "start-running-" + uuid.New().String()
	seedActiveSubscription(t, db, userID)

	terminal, err := createTestTerminal(db, userID, "stopped", time.Now().Add(time.Hour))
	require.NoError(t, err)
	// Put the row in a stopped state with an idle_until set, as it would be
	// after a stop, so we can observe the resume clearing it.
	idle := time.Now().Add(10 * time.Minute)
	require.NoError(t, db.Model(&models.Terminal{}).
		Where("session_id = ?", terminal.SessionID).
		Updates(map[string]any{"state": models.StateStopped, "idle_until": idle}).Error)

	svc := services.NewTerminalTrainerService(db)
	require.NoError(t, svc.StartSession(terminal.SessionID))

	updated, err := svc.GetSessionInfo(terminal.SessionID)
	require.NoError(t, err)
	assert.Equal(t, models.StateRunning, updated.State,
		"resume must transition the row to StateRunning")
	assert.Nil(t, updated.IdleUntil, "resume must clear idle_until")
	assert.Equal(t, newExpiry, updated.ExpiresAt.Unix(),
		"resume must mirror the tt-backend expires_at onto the row")
}
