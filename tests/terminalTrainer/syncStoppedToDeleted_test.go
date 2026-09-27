package terminalTrainer_tests

// A locally-stopped terminal row could never leave that state: the sync's
// stopped-is-authoritative guard refused tt-backend's state wholesale, and
// the orphan sweep never fired because tt keeps (and lists) deleted rows.
// Deadlocked rows sat in 'stopped' for months while the frontend showed
// "Suppression automatique dans moins d'une minute" forever.
//
// The guard's job is narrower than its implementation was: protect a stopped
// row from a RUNNING flap (tt's stop is async; adopting 'running' would
// resurrect the Resume window). A terminal 'deleted' report is not a flap —
// it is the end of the container's life and must win.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	scenarioModels "soli/formations/src/scenarios/models"
	scenarioServices "soli/formations/src/scenarios/services"
	"soli/formations/src/terminalTrainer/models"
	services "soli/formations/src/terminalTrainer/services"
)

// ttServerReportingState fakes tt-backend listing one session in the given
// lifecycle state.
func ttServerReportingState(t *testing.T, sessionID, state string) *httptest.Server {
	t.Helper()
	// Keep the legacy numeric status coherent with the lifecycle state —
	// the sync reads BOTH signals, and an incoherent pair would test a
	// response tt-backend never produces.
	status := 0 // active
	expiresAt := time.Now().Add(time.Hour).Unix()
	if state == "deleted" {
		status = 4
		expiresAt = time.Now().Add(-time.Hour).Unix()
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/1.0/sessions" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions": []map[string]any{
					{
						"id":         sessionID,
						"session_id": sessionID,
						"name":       "tst-lifecycle",
						"status":     status,
						"expires_at": expiresAt,
						"created_at": time.Now().Add(-2 * time.Hour).Unix(),
						"state":      state,
					},
				},
				"count": 1,
			})
			return
		}
		http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}))
}

func seedStoppedTerminal(t *testing.T, sessionID, userID string) {
	t.Helper()
	userKey, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)
	idleUntil := time.Now().Add(-16 * 24 * time.Hour) // long overdue
	require.NoError(t, sharedTestDB.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Deadlocked Terminal",
		State:             models.StateStopped,
		PersistenceMode:   "ephemeral",
		ExpiresAt:         time.Now().Add(-17 * 24 * time.Hour),
		MachineSize:       "S",
		IdleUntil:         &idleUntil,
		UserTerminalKeyID: userKey.ID,
	}).Error)
}

func TestSyncUserSessions_StoppedRowAdoptsTerminalDeletion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	freshTestDB(t)

	sessionID := "sync-dead-" + uuid.New().String()
	userID := "sync-dead-user-" + uuid.New().String()

	srv := ttServerReportingState(t, sessionID, "deleted")
	defer srv.Close()
	configureTTServer(t, srv.URL)

	seedStoppedTerminal(t, sessionID, userID)

	svc := services.NewTerminalTrainerService(sharedTestDB)
	_, err := svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var reloaded models.Terminal
	require.NoError(t, sharedTestDB.Where("session_id = ?", sessionID).First(&reloaded).Error)
	assert.Equal(t, models.StateDeleted, reloaded.State,
		"a stopped row must adopt tt-backend's 'deleted' — otherwise it is "+
			"deadlocked forever (guard blocks the mismatch, orphan sweep "+
			"never fires because tt lists deleted rows)")
}

func TestSyncUserSessions_StoppedRowStillIgnoresRunningFlap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	freshTestDB(t)

	sessionID := "sync-flap-" + uuid.New().String()
	userID := "sync-flap-user-" + uuid.New().String()

	srv := ttServerReportingState(t, sessionID, "running")
	defer srv.Close()
	configureTTServer(t, srv.URL)

	seedStoppedTerminal(t, sessionID, userID)

	svc := services.NewTerminalTrainerService(sharedTestDB)
	_, err := svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var reloaded models.Terminal
	require.NoError(t, sharedTestDB.Where("session_id = ?", sessionID).First(&reloaded).Error)
	assert.Equal(t, models.StateStopped, reloaded.State,
		"the anti-flap guard must survive the fix: a stopped row does not "+
			"adopt a transient 'running' report")
}

// A local 'deleted' row is a tombstone: the sync never brings it back. The
// resurrection below is issue #529 — ocf-core marks an ephemeral terminal
// deleted on stop, but tt-backend used to keep the container and list it as
// 'stopped' (status 0, idle_until 24 h out). The sync then flipped the row
// back to 'stopped' through markSessionStopped, so it held budget again and
// its scenario run read as paused instead of rebuilt.

func seedDeletedTerminal(t *testing.T, sessionID, userID string) {
	t.Helper()
	userKey, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)
	require.NoError(t, sharedTestDB.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Tombstone Terminal",
		State:             models.StateDeleted,
		PersistenceMode:   "ephemeral",
		ExpiresAt:         time.Now().Add(time.Hour),
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}).Error)
}

func TestSyncUserSessions_DeletedRowIgnoresStoppedReport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	freshTestDB(t)

	sessionID := "sync-tomb-stopped-" + uuid.New().String()
	userID := "sync-tomb-stopped-user-" + uuid.New().String()

	srv := syncSessionTTServer(t, sessionID, "stopped", "ephemeral",
		time.Now().Add(time.Hour).Unix(), time.Now().Add(24*time.Hour).Unix())
	defer srv.Close()
	configureTTServer(t, srv.URL)

	seedDeletedTerminal(t, sessionID, userID)

	_, err := services.NewTerminalTrainerService(sharedTestDB).SyncUserSessions(userID)
	require.NoError(t, err)

	var reloaded models.Terminal
	require.NoError(t, sharedTestDB.Where("session_id = ?", sessionID).First(&reloaded).Error)
	assert.Equal(t, models.StateDeleted, reloaded.State,
		"a deleted row is a tombstone: tt-backend listing it as stopped must "+
			"not bring it back through markSessionStopped")
}

func TestSyncUserSessions_DeletedRowIgnoresRunningReport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	freshTestDB(t)

	sessionID := "sync-tomb-running-" + uuid.New().String()
	userID := "sync-tomb-running-user-" + uuid.New().String()

	srv := ttServerReportingState(t, sessionID, "running")
	defer srv.Close()
	configureTTServer(t, srv.URL)

	seedDeletedTerminal(t, sessionID, userID)

	_, err := services.NewTerminalTrainerService(sharedTestDB).SyncUserSessions(userID)
	require.NoError(t, err)

	var reloaded models.Terminal
	require.NoError(t, sharedTestDB.Where("session_id = ?", sessionID).First(&reloaded).Error)
	assert.Equal(t, models.StateDeleted, reloaded.State,
		"a deleted row is a tombstone: a container tt-backend still runs "+
			"(DeleteSession whose tt call failed) must not come back as running")
}

// The whole #529 path: an ephemeral stop, a tt-backend that keeps listing the
// container as stopped, then a sync. The terminal must stay a tombstone and
// the scenario run on it must be rebuilt, not resumed as paused.
func TestStopThenSync_EphemeralTerminalStaysDeletedAndItsRunIsRebuilt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	db := freshTestDB(t)
	userID := "stop-sync-tomb-user-" + uuid.New().String()
	seedActiveSubscription(t, db, userID)

	terminal, err := createTestTerminal(db, userID, "running", time.Now().Add(time.Hour))
	require.NoError(t, err)
	terminal.PersistenceMode = "ephemeral"
	require.NoError(t, db.Save(terminal).Error)

	idleUntil := time.Now().Add(24 * time.Hour)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/stop"):
			_ = json.NewEncoder(w).Encode(map[string]any{"idle_until": idleUntil.UTC().Format(time.RFC3339)})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sessions"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions": []map[string]any{{
					"id":               terminal.SessionID,
					"session_id":       terminal.SessionID,
					"name":             "tst",
					"status":           0, // tt keeps a stopped session active
					"expires_at":       time.Now().Add(time.Hour).Unix(),
					"created_at":       time.Now().Add(-time.Hour).Unix(),
					"state":            "stopped",
					"persistence_mode": "ephemeral",
					"idle_until":       idleUntil.Unix(),
				}},
				"count": 1,
			})
		default:
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	configureTTServer(t, srv.URL)

	svc := services.NewTerminalTrainerService(db)
	require.NoError(t, svc.StopSession(terminal.SessionID))
	_, err = svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var reloaded models.Terminal
	require.NoError(t, db.Where("session_id = ?", terminal.SessionID).First(&reloaded).Error)
	assert.Equal(t, models.StateDeleted, reloaded.State,
		"the sync after an ephemeral stop must leave the tombstone deleted")
	assert.False(t, reloaded.HoldsContainer(),
		"a stopped ephemeral terminal holds no container")

	run := &scenarioModels.ScenarioSession{Status: "active", TerminalSessionID: &terminal.SessionID}
	assert.Equal(t, scenarioServices.ResumeModeRebuild, scenarioServices.RunResumeMode(run, &reloaded, false),
		"a normal run whose ephemeral terminal was stopped is rebuilt, not paused")
}
