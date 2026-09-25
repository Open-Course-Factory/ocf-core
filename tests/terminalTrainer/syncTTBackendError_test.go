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

// tt-backend lists a user's sessions by API key, so one unprefixed listing is
// complete. Listing once per instance type in the user's history bought
// nothing, and broke the sync for good once a distribution was retired: its
// prefix 404s, and a failed listing must fail the sync. The retired row is
// then just a row tt-backend no longer lists — handled like any other.
func TestSyncUserSessions_UnknownDistributionInHistory_StillSyncs(t *testing.T) {
	freshTestDB(t)
	userID := "sync-retired-" + uuid.New().String()
	userKey, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)

	liveID := "sync-retired-live-" + uuid.New().String()
	retiredID := "sync-retired-old-" + uuid.New().String()
	for _, terminal := range []models.Terminal{
		{SessionID: liveID, InstanceType: "", State: models.StateRunning},
		{SessionID: retiredID, InstanceType: "retired-distro", State: models.StateStopped},
	} {
		terminal.UserID = userID
		terminal.Name = terminal.SessionID
		terminal.PersistenceMode = "persistent"
		terminal.ExpiresAt = time.Now().Add(time.Hour)
		terminal.MachineSize = "S"
		terminal.UserTerminalKeyID = userKey.ID
		require.NoError(t, sharedTestDB.Create(&terminal).Error)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/1.0/sessions" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions": []map[string]any{{
					"id": liveID, "session_id": liveID, "name": liveID,
					"status": 0, "state": "running", "persistence_mode": "persistent",
					"expires_at": time.Now().Add(time.Hour).Unix(),
				}},
				"count": 1, "include_expired": true, "limit": 1000,
			})
			return
		}
		// /1.0/retired-distro/sessions and anything else: tt-backend has no
		// such distribution any more.
		http.Error(w, "unknown distribution", http.StatusNotFound)
	}))
	defer srv.Close()
	configureTTServer(t, srv.URL)

	_, err = services.NewTerminalTrainerService(sharedTestDB).SyncUserSessions(userID)
	require.NoError(t, err, "a retired distribution in the user's history must not fail their sync")

	var live, retired models.Terminal
	require.NoError(t, sharedTestDB.Where("session_id = ?", liveID).First(&live).Error)
	require.NoError(t, sharedTestDB.Where("session_id = ?", retiredID).First(&retired).Error)
	assert.Equal(t, models.StateRunning, live.State, "the listed terminal stays running")
	assert.Equal(t, models.StateDeleted, retired.State,
		"a row a complete listing no longer returns is marked deleted, whatever its distribution")
}
