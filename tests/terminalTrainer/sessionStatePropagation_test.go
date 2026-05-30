// tests/terminalTrainer/sessionStatePropagation_test.go
//
// Bug under test: ocf-core caches the session lifecycle from tt-backend with
// no invalidation. When tt-backend auto-stops a persistent session (sets
// sessions.state='stopped'), ocf-core's terminals.state stays 'running',
// so the frontend keeps showing "Session expirée" instead of a "Resume"
// affordance for a still-resumable session.
//
// Root cause has three legs in ocf-core:
//
//   1. The DTO that maps tt-backend's /1.0/sessions response
//      (TerminalTrainerSession) has NO state / persistence_mode / idle_until
//      fields, so even if tt-backend sends them they are silently dropped.
//
//   2. SyncUserSessions never writes localSession.State (or
//      PersistenceMode / IdleUntil) — only localSession.Status.
//
// These two tests pin both legs. They MUST be red until both are fixed.
// A passing test 3 will require fixing both legs (and the tt-backend leg —
// covered in tt-backend's own test).
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// fieldByName returns the value of struct field `name` on `v`, or nil if the
// field does not exist on the type. Used so this test file compiles even
// before the DTO is patched — the assertion fails cleanly at runtime when
// the field is missing, rather than blocking Test 3 with a build error.
func fieldByName(v any, name string) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	f := rv.FieldByName(name)
	if !f.IsValid() {
		return nil
	}
	return f.Interface()
}

// ---------------------------------------------------------------------------
// Test 2 — TerminalTrainerSession round-trips state / persistence_mode /
//          idle_until from the JSON tt-backend SHOULD return.
//
// Currently the DTO drops those fields silently. This is the contract failure
// upstream of the cache-staleness bug — once ocf-core's DTO can hold the
// fields, sync-time propagation becomes possible.
// ---------------------------------------------------------------------------

func TestTerminalTrainerSession_DeserializesLifecycleFields(t *testing.T) {
	const idleUntilTS int64 = 1893456000 // arbitrary future Unix timestamp

	// Payload modeled on what tt-backend SHOULD include in /1.0/sessions
	// for a persistent session that has been auto-stopped.
	payload := []byte(`{
		"id": "sess-abc-123",
		"session_id": "sess-abc-123",
		"name": "tst-persistent",
		"status": 0,
		"expires_at": 1893460000,
		"created_at": 1893450000,
		"ip": "10.0.0.42",
		"state": "stopped",
		"persistence_mode": "persistent",
		"idle_until": 1893456000
	}`)

	var s dto.TerminalTrainerSession
	err := json.Unmarshal(payload, &s)
	require.NoError(t, err, "unmarshalling a tt-backend session payload must not error")

	// Sanity: existing fields still parse.
	assert.Equal(t, "sess-abc-123", s.SessionID, "session_id must round-trip")

	// The bug: these three fields are dropped because they do not exist on
	// the struct. The assertions name the production-side fields that need
	// to be added (State, PersistenceMode, IdleUntil). Looked up by name so
	// this file compiles before the fix; will assert real values after.
	assert.Equal(t, models.StateStopped, fieldByName(&s, "State"),
		"TerminalTrainerSession must expose a State field that round-trips from JSON; tt-backend's auto-stop signal is lost otherwise")
	assert.Equal(t, "persistent", fieldByName(&s, "PersistenceMode"),
		"TerminalTrainerSession must expose a PersistenceMode field that round-trips from JSON; frontend cannot offer Resume without it")
	assert.Equal(t, idleUntilTS, fieldByName(&s, "IdleUntil"),
		"TerminalTrainerSession must expose an IdleUntil int64 field that round-trips from JSON; reaper deadline is lost otherwise")
}

// ---------------------------------------------------------------------------
// Test 3 — SyncUserSessions propagates state / persistence_mode / idle_until
//          from tt-backend into the local terminals table.
//
// This is the end-to-end contract test. Even if the DTO is fixed (Test 2),
// SyncUserSessions today only ever sets localSession.Status, never
// localSession.State / PersistenceMode / IdleUntil. So even a perfectly
// deserialised tt-backend payload would not be reflected in the cache the
// frontend reads.
//
// The test stands up a fake tt-backend that returns a single session with
// state="stopped", persistence_mode="persistent", idle_until=<future>.
// It seeds a local terminal row with State="running" / Status="active",
// runs SyncUserSessions, then re-reads the row from the canonical DB and
// asserts the lifecycle fields were updated.
// ---------------------------------------------------------------------------

// stoppedSessionTTServer mimics tt-backend's /1.0/sessions response shape
// AFTER the tt-backend leg of the fix lands. The test consumes the response
// from ocf-core's perspective.
func stoppedSessionTTServer(t *testing.T, sessionID string, idleUntilTS int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Match the path ocf-core's service hits: GET /<version>/sessions
		if r.Method == http.MethodGet && r.URL.Path == "/1.0/sessions" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions": []map[string]any{
					{
						"id":               sessionID,
						"session_id":       sessionID,
						"name":             "tst-persistent",
						"status":           0, // SessionStatusActive
						"expires_at":       time.Now().Add(time.Hour).Unix(),
						"created_at":       time.Now().Add(-time.Hour).Unix(),
						"ip":               "10.0.0.42",
						"state":            "stopped",
						"persistence_mode": "persistent",
						"idle_until":       idleUntilTS,
					},
				},
				"count":           1,
				"api_key_id":      0,
				"include_expired": true,
				"limit":           1000,
			})
			return
		}
		http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}))
}

func TestSyncUserSessions_PropagatesStateFromTTBackend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	sessionID := "sync-state-session-" + uuid.New().String()
	idleUntilTS := time.Now().Add(30 * time.Minute).Unix()

	srv := stoppedSessionTTServer(t, sessionID, idleUntilTS)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "sync-state-user-" + uuid.New().String()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Seed a local terminal row in the OPPOSITE state to what tt-backend
	// will report. Sync must overwrite it.
	local := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Test Terminal",
		State:             models.StateRunning, // canonical SSOT — sync must flip to "stopped"
		PersistenceMode:   "ephemeral",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(local).Error)

	svc := services.NewTerminalTrainerService(db)

	_, syncErr := svc.SyncUserSessions(userID)
	require.NoError(t, syncErr, "SyncUserSessions must not error against a healthy fake tt-backend")

	// Re-read from the canonical store. Asserting on the persisted row is
	// what the frontend will read — testing the service return value would
	// not catch silent field-loss at the repository.UpdateTerminalSession
	// boundary.
	var reloaded models.Terminal
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&reloaded).Error)

	assert.Equal(t, models.StateStopped, reloaded.State,
		"terminals.state must reflect tt-backend's state field; otherwise the frontend keeps showing 'Session expirée' for resumable sessions")
	assert.Equal(t, "persistent", reloaded.PersistenceMode,
		"terminals.persistence_mode must reflect tt-backend's persistence_mode field; required for the Resume affordance")
	require.NotNil(t, reloaded.IdleUntil,
		"terminals.idle_until must be populated from tt-backend's idle_until field")
	assert.Equal(t, idleUntilTS, reloaded.IdleUntil.Unix(),
		"terminals.idle_until timestamp must match tt-backend's idle_until")
}
