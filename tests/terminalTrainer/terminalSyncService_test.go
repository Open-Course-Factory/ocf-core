package terminalTrainer_tests

// These tests pin the session-reconciliation contract owned by
// terminalSyncService (extracted from terminalTrainerService, MR A3/8).
// They exercise SyncUserSessions through the public TerminalTrainerService
// facade against a stub tt-backend and assert on the observable DB row state
// AFTER the sync — so they survive the extraction and any future move,
// provided the behavior is preserved.
//
// They complement the existing sync suites rather than duplicate them:
//   - sessionStatePropagation_test.go pins state propagation on an existing row
//   - syncStoppedPersistentExpiry_test.go pins the stopped/idle_until path
//   - stopSessionPersistence_test.go pins StopSession + the stopped-missing case
//
// What is uniquely pinned here:
//   - a missing local row is CREATED from a running API session (5a create)
//   - a local row absent from the API list is soft-deleted (5b reap)
//   - a stopped API session propagates state + idle_until into an existing row
//     (re-asserted through the facade to lock the create/sync split)

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// syncStubBackend serves the supplied sessions at GET /<version>/sessions and
// absorbs every other call (stop/start/delete). The provider is read on each
// request so a test can mutate the returned set between syncs.
func syncStubBackend(t *testing.T, provider func() []dto.TerminalTrainerSession) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sessions") {
			s := provider()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(dto.TerminalTrainerSessionsResponse{
				Sessions:       s,
				Count:          len(s),
				IncludeExpired: true,
				Limit:          1000,
			})
			return
		}
		// Absorb stop/start/delete and anything else with a benign 200.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
}

// TestSyncUserSessions_MissingLocalRow_CreatedFromAPI pins step 5a's create
// branch: a running API session with no local counterpart materialises a new
// local row (with the size denorm from BuildTerminalFromAPISession).
func TestSyncUserSessions_MissingLocalRow_CreatedFromAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	userID := "sync-create-user"
	sessionID := "sess-create-1"

	sessions := []dto.TerminalTrainerSession{
		{
			SessionID:   sessionID,
			Status:      0, // active
			ExpiresAt:   time.Now().Add(1 * time.Hour).Unix(),
			State:       models.StateRunning,
			MachineSize: "XS",
		},
	}

	srv := syncStubBackend(t, func() []dto.TerminalTrainerSession { return sessions })
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	svc := services.NewTerminalTrainerService(db)

	_, err = svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var got models.Terminal
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&got).Error,
		"a running API session with no local row must be created locally")
	assert.Equal(t, models.StateRunning, got.State)
	assert.Equal(t, userID, got.UserID)
}

// TestSyncUserSessions_OrphanLocalRow_SoftDeleted pins step 5b's reap branch:
// a running local row that no longer appears in the API list is marked
// StateDeleted (the container has been reaped on tt-backend).
func TestSyncUserSessions_OrphanLocalRow_SoftDeleted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	userID := "sync-reap-user"
	sessionID := "sess-reap-1"

	// API returns an empty session list — the local row is now an orphan.
	srv := syncStubBackend(t, func() []dto.TerminalTrainerSession { return nil })
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	require.NoError(t, db.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		State:             models.StateRunning,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}).Error)

	svc := services.NewTerminalTrainerService(db)

	_, err = svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var got models.Terminal
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&got).Error)
	assert.Equal(t, models.StateDeleted, got.State,
		"a local row absent from the API list must be soft-deleted")
}

// TestSyncUserSessions_StoppedAPISession_PropagatesStateAndIdleUntil pins the
// stopped-transition path through the facade: an existing running row whose
// API counterpart reports state=stopped adopts StateStopped and the
// tt-backend idle_until as its ExpiresAt (via the shared markSessionStopped).
func TestSyncUserSessions_StoppedAPISession_PropagatesStateAndIdleUntil(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	userID := "sync-stop-user"
	sessionID := "sess-stop-1"

	idleUntil := time.Now().Add(20 * time.Minute).Truncate(time.Second)
	sessions := []dto.TerminalTrainerSession{
		{
			SessionID:       sessionID,
			Status:          0,                                       // still listed as active
			ExpiresAt:       time.Now().Add(-1 * time.Minute).Unix(), // stale create deadline
			State:           models.StateStopped,
			PersistenceMode: "persistent",
			IdleUntil:       idleUntil.Unix(),
		},
	}

	srv := syncStubBackend(t, func() []dto.TerminalTrainerSession { return sessions })
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	require.NoError(t, db.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		State:             models.StateRunning,
		ExpiresAt:         time.Now().Add(-1 * time.Minute),
		PersistenceMode:   "persistent",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}).Error)

	svc := services.NewTerminalTrainerService(db)

	_, err = svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var got models.Terminal
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&got).Error)
	assert.Equal(t, models.StateStopped, got.State)
	assert.WithinDuration(t, idleUntil, got.ExpiresAt, 2*time.Second,
		"stopped row must extend ExpiresAt to the tt-backend idle_until, not keep the stale create deadline")
}
