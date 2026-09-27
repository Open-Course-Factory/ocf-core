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
	"sync"
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

// ttServerReportingState fakes tt-backend listing one session with the given
// legacy status and lifecycle state. The sync reads BOTH signals, so callers
// pass a pair tt-backend really produces. A non-zero status is past its expiry.
func ttServerReportingState(t *testing.T, sessionID, state string, status int) *httptest.Server {
	t.Helper()
	expiresAt := time.Now().Add(time.Hour).Unix()
	if status != 0 {
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

func seedSyncTerminal(t *testing.T, sessionID, userID string, state models.TerminalState, expiresAt time.Time) {
	t.Helper()
	userKey, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)
	require.NoError(t, sharedTestDB.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Sync Terminal",
		State:             state,
		PersistenceMode:   "ephemeral",
		ExpiresAt:         expiresAt,
		MachineSize:       "S",
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

	srv := ttServerReportingState(t, sessionID, "deleted", 4)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	seedSyncTerminal(t, sessionID, userID, models.StateStopped, time.Now().Add(-17*24*time.Hour))

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

	srv := ttServerReportingState(t, sessionID, "running", 0)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	seedSyncTerminal(t, sessionID, userID, models.StateStopped, time.Now().Add(-17*24*time.Hour))

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

	seedSyncTerminal(t, sessionID, userID, models.StateDeleted, time.Now().Add(time.Hour))

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

	srv := ttServerReportingState(t, sessionID, "running", 0)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	seedSyncTerminal(t, sessionID, userID, models.StateDeleted, time.Now().Add(time.Hour))

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

// The status-derived write turns a running row past its expiry into deleted,
// and the lifecycle state write then restores running. tt-backend answers that
// pair (status 1, state running) for a clock-expired session its reaper has not
// reached yet. The tombstone rule must judge the row as it was before the pass;
// judged on the intermediate deleted, it would bury a live persistent session
// and refuse the stopped report that follows its auto-stop, losing Resume.
func TestSyncUserSessions_ClockExpiredRunningRowStaysRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	freshTestDB(t)

	sessionID := "sync-clock-expired-" + uuid.New().String()
	userID := "sync-clock-expired-user-" + uuid.New().String()

	srv := ttServerReportingState(t, sessionID, "running", 1)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	seedSyncTerminal(t, sessionID, userID, models.StateRunning, time.Now().Add(time.Hour))

	_, err := services.NewTerminalTrainerService(sharedTestDB).SyncUserSessions(userID)
	require.NoError(t, err)

	var reloaded models.Terminal
	require.NoError(t, sharedTestDB.Where("session_id = ?", sessionID).First(&reloaded).Error)
	assert.Equal(t, models.StateRunning, reloaded.State,
		"a running row tt reports as status 1 + state running stays running: "+
			"the tombstone rule reads the state before the pass, not the "+
			"intermediate deleted the status write left")
}

// A revoked row whose tt expiry falls in the current second: tt still answers
// status 0 (it compares whole seconds), so the sync's expiry check fires before
// the stopped report is read. Revoked is authoritative: the expiry check must
// not relabel it as expired, and markSessionStopped must not hand the revoked
// user a resumable session back.
func TestSyncUserSessions_RevokedRowInItsExpirySecondStaysRevoked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	freshTestDB(t)

	sessionID := "sync-revoked-expiry-" + uuid.New().String()
	userID := "sync-revoked-expiry-user-" + uuid.New().String()

	srv := syncSessionTTServer(t, sessionID, "stopped", "persistent",
		time.Now().Unix(), time.Now().Add(24*time.Hour).Unix())
	defer srv.Close()
	configureTTServer(t, srv.URL)

	seedSyncTerminal(t, sessionID, userID, models.StateRevoked, time.Now().Add(time.Hour))

	_, err := services.NewTerminalTrainerService(sharedTestDB).SyncUserSessions(userID)
	require.NoError(t, err)

	var reloaded models.Terminal
	require.NoError(t, sharedTestDB.Where("session_id = ?", sessionID).First(&reloaded).Error)
	assert.Equal(t, models.StateRevoked, reloaded.State,
		"a revoked row stays revoked: stopped would be a resumable session "+
			"holding budget for a revoked user, deleted would relabel a "+
			"billing revocation as a plain expiry")
}

// Marking a row deleted does not prove tt destroyed its container: DeleteSession
// and an ephemeral StopSession mark the tombstone even when the tt call fails.
// The sync used to revive such a row, which by accident let the learner delete
// it again. Now that a tombstone stays buried, the sync itself retries the tt
// DELETE whenever tt still lists the container as live (status 0), or the
// orphan runs unseen until tt's expiry and may hold the key's tt budget. A
// session tt already reports expired or deleted is left alone: tt lists those
// on every pass (include_expired), so deleting them again would be noise. That
// includes status 0 with state deleted, which tt answers when its final write
// failed after the container was destroyed; a DELETE there only earns a 409. A
// retry tt refuses is not the pass's failure: the row stays buried and the
// next pass tries again.
func TestSyncUserSessions_TombstoneRetriesTheTTDeleteOnlyWhileTheContainerIsLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	for _, tc := range []struct {
		name        string
		status      int
		state       string
		wantDelete  bool
		deleteFails bool
	}{
		{"running", 0, "running", true, false},
		{"stopped", 0, "stopped", true, false},
		{"retry refused by tt", 0, "running", true, true},
		{"status 0 but state deleted", 0, "deleted", false, false},
		{"deleted", 4, "deleted", false, false},
		{"clock-expired", 1, "running", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			freshTestDB(t)

			sessionID := "sync-tomb-retry-" + uuid.New().String()
			userID := "sync-tomb-retry-user-" + uuid.New().String()

			var mu sync.Mutex
			var deletes []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/sessions/"+sessionID):
					mu.Lock()
					deletes = append(deletes, sessionID)
					mu.Unlock()
					if tc.deleteFails {
						http.Error(w, `{"error":"backend unavailable"}`, http.StatusInternalServerError)
						return
					}
					_, _ = w.Write([]byte(`{}`))
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sessions"):
					expiresAt := time.Now().Add(time.Hour).Unix()
					if tc.status != 0 {
						expiresAt = time.Now().Add(-time.Hour).Unix()
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"sessions": []map[string]any{{
							"id":         sessionID,
							"session_id": sessionID,
							"name":       "tst",
							"status":     tc.status,
							"expires_at": expiresAt,
							"created_at": time.Now().Add(-2 * time.Hour).Unix(),
							"state":      tc.state,
						}},
						"count": 1,
					})
				default:
					http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
				}
			}))
			defer srv.Close()
			configureTTServer(t, srv.URL)

			seedSyncTerminal(t, sessionID, userID, models.StateDeleted, time.Now().Add(time.Hour))

			_, err := services.NewTerminalTrainerService(sharedTestDB).SyncUserSessions(userID)
			require.NoError(t, err, "a retry DELETE tt refuses does not fail the sync pass")

			var reloaded models.Terminal
			require.NoError(t, sharedTestDB.Where("session_id = ?", sessionID).First(&reloaded).Error)
			assert.Equal(t, models.StateDeleted, reloaded.State, "a tombstone stays deleted")

			mu.Lock()
			defer mu.Unlock()
			if tc.wantDelete {
				assert.Equal(t, []string{sessionID}, deletes,
					"tt still lists the tombstone's container as live: the sync must retry the tt DELETE once")
			} else {
				assert.Empty(t, deletes,
					"tt already reports the container expired or deleted: the sync must not delete it again")
			}
		})
	}
}
