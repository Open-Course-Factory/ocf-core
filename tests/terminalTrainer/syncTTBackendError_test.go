package terminalTrainer_tests

// SyncUserSessions treats tt-backend's listing as the truth about which
// containers exist: a local row absent from it is marked deleted. That is only
// sound when the listing is complete. A listing that failed — tt-backend down,
// a 500, one instance type out of several unreachable — says nothing about the
// rows it did not return, yet the per-instance-type fetch logged the error and
// carried on, handing the sync an empty (or partial) listing with no error.
// Every terminal of the user was then marked deleted, and the scenario zombie
// sweep abandoned their runs: one tt-backend outage ended every learner's run.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/models"
	services "soli/formations/src/terminalTrainer/services"
)

// seedSyncErrorTerminals gives userID a live terminal and a paused persistent
// one, both of the given instance type, and returns their session ids.
func seedSyncErrorTerminals(t *testing.T, userID string, instanceTypes ...string) []string {
	t.Helper()
	userKey, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)

	var ids []string
	for _, instanceType := range instanceTypes {
		for _, state := range []models.TerminalState{models.StateRunning, models.StateStopped} {
			id := "sync-err-" + string(state) + "-" + uuid.New().String()
			require.NoError(t, sharedTestDB.Create(&models.Terminal{
				SessionID:         id,
				UserID:            userID,
				Name:              id,
				State:             state,
				PersistenceMode:   "persistent",
				ExpiresAt:         time.Now().Add(time.Hour),
				InstanceType:      instanceType,
				MachineSize:       "S",
				UserTerminalKeyID: userKey.ID,
			}).Error)
			ids = append(ids, id)
		}
	}
	return ids
}

// assertStatesUnchanged checks that no terminal left the state it was seeded in.
func assertStatesUnchanged(t *testing.T, ids []string) {
	t.Helper()
	for _, id := range ids {
		var terminal models.Terminal
		require.NoError(t, sharedTestDB.Where("session_id = ?", id).First(&terminal).Error)
		assert.NotEqual(t, models.StateDeleted, terminal.State,
			"%s must keep its state: a failed listing proves nothing about it", id)
	}
}

func TestSyncUserSessions_TTBackendError_DeletesNothing(t *testing.T) {
	freshTestDB(t)
	userID := "sync-err-500-" + uuid.New().String()
	ids := seedSyncErrorTerminals(t, userID, "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "tt-backend is down", http.StatusInternalServerError)
	}))
	defer srv.Close()
	configureTTServer(t, srv.URL)

	_, err := services.NewTerminalTrainerService(sharedTestDB).SyncUserSessions(userID)

	assert.Error(t, err, "a sync that could not list the sessions must say so")
	assertStatesUnchanged(t, ids)
}

func TestSyncUserSessions_TTBackendUnreachable_DeletesNothing(t *testing.T) {
	freshTestDB(t)
	userID := "sync-err-refused-" + uuid.New().String()
	ids := seedSyncErrorTerminals(t, userID, "")

	// A server that is gone: the port refuses connections.
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	configureTTServer(t, url)

	_, err := services.NewTerminalTrainerService(sharedTestDB).SyncUserSessions(userID)

	assert.Error(t, err, "a sync that could not reach tt-backend must say so")
	assertStatesUnchanged(t, ids)
}

// The user has terminals of two instance types, so the sync lists two paths.
// The default one answers and lists its own sessions; the other fails. The
// terminals of the failed type are missing from the combined listing only
// because their listing failed, not because tt-backend reaped them.
func TestSyncUserSessions_PartialInstanceTypeFailure_DeletesNothing(t *testing.T) {
	freshTestDB(t)
	userID := "sync-err-partial-" + uuid.New().String()
	ids := seedSyncErrorTerminals(t, userID, "", "ubuntu")
	defaultIDs, ubuntuIDs := ids[:2], ids[2:]

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/sessions":
			sessions := make([]map[string]any, 0, len(defaultIDs))
			for _, id := range defaultIDs {
				sessions = append(sessions, map[string]any{
					"id": id, "session_id": id, "name": id,
					"status": 0, "state": "running", "persistence_mode": "persistent",
					"expires_at": time.Now().Add(time.Hour).Unix(),
				})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions": sessions, "count": len(sessions), "include_expired": true, "limit": 1000,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/ubuntu/sessions":
			http.Error(w, "this backend is down", http.StatusBadGateway)
		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	configureTTServer(t, srv.URL)

	_, err := services.NewTerminalTrainerService(sharedTestDB).SyncUserSessions(userID)

	assert.Error(t, err, "a sync whose listing is incomplete must say so rather than act on it")
	assertStatesUnchanged(t, ubuntuIDs)
}
