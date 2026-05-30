// tests/terminalTrainer/syncStoppedPersistentExpiry_test.go
//
// Sync-path contract under D6' (supersedes D6, locked 2026-05-28): "a stop
// is a stop". When tt-backend auto-stops a session, SyncUserSessions reaches
// the local row through step 5a and routes through the SSOT helper
// markSessionStopped, which:
//   - sets state="stopped"
//   - extends expires_at to idle_until (with plan-derived fallback)
//   - populates IdleUntil
//
// The persistence_mode distinction is NOT honored here — auto-stopped
// ephemeral goes through the same transition as auto-stopped persistent.
// The slot stays reserved until sync's step 5b observes that tt-backend
// has lost the container (then marks the local row deleted).
//
// Original motivation (pre-D6'): commit ae8aba7 introduced the persistent-only
// extension; commit c9814d1 added the helper. D6' generalises both to
// "any stop", killing the persistence_mode branch.
package terminalTrainer_tests

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

	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// syncSessionTTServer returns a /1.0/sessions response with the supplied
// payload fields. It mirrors the convention of stoppedSessionTTServer in
// sessionStatePropagation_test.go but exposes idle_until as a control knob
// (set <= 0 to omit the field, otherwise emit it as a unix timestamp).
//
// Optional state/persistence_mode/expires_at let each test pin a precise
// shape.
func syncSessionTTServer(
	t *testing.T,
	sessionID string,
	apiState string,
	persistenceMode string,
	expiresAt int64,
	idleUntilTS int64,
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sessions") {
			w.Header().Set("Content-Type", "application/json")
			row := map[string]any{
				"id":               sessionID,
				"session_id":       sessionID,
				"name":             "tst",
				"status":           0, // SessionStatusActive — keep sync on the "running side" path that propagates state
				"expires_at":       expiresAt,
				"created_at":       time.Now().Add(-time.Hour).Unix(),
				"state":            apiState,
				"persistence_mode": persistenceMode,
			}
			if idleUntilTS > 0 {
				row["idle_until"] = idleUntilTS
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions":        []map[string]any{row},
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

// ---------------------------------------------------------------------------
// (a) Auto-stopped: sync must extend ExpiresAt to idle_until — for BOTH
//     persistence modes (D6': "a stop is a stop").
// ---------------------------------------------------------------------------

func TestSyncUserSessions_AutoStopped_ExtendsExpiresAtToIdleUntil(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	cases := []struct {
		name string
		mode string
	}{
		{"persistent", "persistent"},
		{"ephemeral", "ephemeral"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := "sync-auto-stop-" + tc.mode + "-" + uuid.New().String()
			idleUntil := time.Now().Add(30 * time.Minute).Truncate(time.Second)
			originalExpiry := time.Now().Add(-time.Minute) // already past

			srv := syncSessionTTServer(t, sessionID,
				"stopped", tc.mode,
				originalExpiry.Unix(), // tt-backend echoes its own (past) expiry
				idleUntil.Unix(),
			)
			defer srv.Close()
			configureTTServer(t, srv.URL)

			db := freshTestDB(t)
			userID := "owner-auto-stop-" + tc.mode + "-" + uuid.New().String()
			userKey, err := createTestUserKey(db, userID)
			require.NoError(t, err)

			// Local row: still in "running" with the original creation deadline that
			// has just elapsed. This is exactly the state ocf-core was in when
			// tt-backend reaped the session.
			local := &models.Terminal{
				SessionID:         sessionID,
				UserID:            userID,
				Name:              "Test Terminal",
				State:             models.StateRunning,
				PersistenceMode:   tc.mode,
				ExpiresAt:         originalExpiry,
				InstanceType:      "",
				MachineSize:       "S",
				UserTerminalKeyID: userKey.ID,
			}
			require.NoError(t, db.Create(local).Error)

			svc := services.NewTerminalTrainerService(db)
			_, err = svc.SyncUserSessions(userID)
			require.NoError(t, err)

			var reloaded models.Terminal
			require.NoError(t, db.Where("session_id = ?", sessionID).First(&reloaded).Error)
			assert.Equal(t, models.StateStopped, reloaded.State,
				"sync must propagate tt-backend's stopped state (%s)", tc.mode)
			assert.Equal(t, tc.mode, reloaded.PersistenceMode)

			// ExpiresAt must be aligned with idle_until (tolerate 1s clock drift).
			delta := reloaded.ExpiresAt.Sub(idleUntil)
			if delta < -time.Second || delta > time.Second {
				t.Errorf("expected ExpiresAt ~= idleUntil (%v), got %v (delta=%v) — sync must extend expires_at on every stopped row (D6') so OccupiesSlotScope keeps counting them",
					idleUntil, reloaded.ExpiresAt, delta)
			}
			assert.True(t, reloaded.ExpiresAt.After(time.Now()),
				"after sync, stopped %s ExpiresAt must be in the future", tc.mode)
		})
	}
}

// ---------------------------------------------------------------------------
// (b) Auto-stopped, idle_until absent: falls back to plan duration —
//     BOTH persistence modes.
// ---------------------------------------------------------------------------

func TestSyncUserSessions_AutoStopped_IdleUntilZero_UsesPlanFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	cases := []struct {
		name string
		mode string
	}{
		{"persistent", "persistent"},
		{"ephemeral", "ephemeral"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := "sync-auto-stop-no-idle-" + tc.mode + "-" + uuid.New().String()
			originalExpiry := time.Now().Add(-time.Minute)

			srv := syncSessionTTServer(t, sessionID,
				"stopped", tc.mode,
				originalExpiry.Unix(),
				0, // omit idle_until — exercise the plan-derived fallback
			)
			defer srv.Close()
			configureTTServer(t, srv.URL)

			db := freshTestDB(t)
			userID := "owner-auto-stop-fallback-" + tc.mode + "-" + uuid.New().String()
			userKey, err := createTestUserKey(db, userID)
			require.NoError(t, err)

			local := &models.Terminal{
				SessionID:         sessionID,
				UserID:            userID,
				Name:              "Test Terminal",
				State:             models.StateRunning,
				PersistenceMode:   tc.mode,
				ExpiresAt:         originalExpiry,
				InstanceType:      "",
				MachineSize:       "S",
				UserTerminalKeyID: userKey.ID,
			}
			require.NoError(t, db.Create(local).Error)
			// Plan dictates the fallback: 60 minutes.
			seedPlanForTerminal(t, db, local, 60)

			before := time.Now()
			svc := services.NewTerminalTrainerService(db)
			_, err = svc.SyncUserSessions(userID)
			require.NoError(t, err)
			after := time.Now()

			var reloaded models.Terminal
			require.NoError(t, db.Where("session_id = ?", sessionID).First(&reloaded).Error)
			assert.Equal(t, models.StateStopped, reloaded.State)

			expectedLow := before.Add(60 * time.Minute).Add(-time.Second)
			expectedHigh := after.Add(60 * time.Minute).Add(5 * time.Second)
			if reloaded.ExpiresAt.Before(expectedLow) || reloaded.ExpiresAt.After(expectedHigh) {
				t.Errorf("expected ExpiresAt in [%v, %v], got %v — sync must fall back to plan.MaxSessionDurationMinutes when tt-backend omits idle_until (%s)",
					expectedLow, expectedHigh, reloaded.ExpiresAt, tc.mode)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// (d) Already-stopped with expires_at == idle_until: idempotent — BOTH modes.
// ---------------------------------------------------------------------------

func TestSyncUserSessions_AlreadyStopped_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	cases := []struct {
		name string
		mode string
	}{
		{"persistent", "persistent"},
		{"ephemeral", "ephemeral"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := "sync-idempotent-" + tc.mode + "-" + uuid.New().String()
			idleUntil := time.Now().Add(30 * time.Minute).Truncate(time.Second)

			srv := syncSessionTTServer(t, sessionID,
				"stopped", tc.mode,
				idleUntil.Unix(), // tt-backend's expires_at echoes the same window
				idleUntil.Unix(),
			)
			defer srv.Close()
			configureTTServer(t, srv.URL)

			db := freshTestDB(t)
			userID := "owner-idempotent-" + tc.mode + "-" + uuid.New().String()
			userKey, err := createTestUserKey(db, userID)
			require.NoError(t, err)

			// Local row already in the post-fix steady state: stopped with
			// expires_at = idle_until. Sync must be a no-op (no spurious writes).
			idleUntilLocal := idleUntil.Local()
			local := &models.Terminal{
				SessionID:         sessionID,
				UserID:            userID,
				Name:              "Test Terminal",
				State:             models.StateStopped,
				PersistenceMode:   tc.mode,
				ExpiresAt:         idleUntilLocal,
				IdleUntil:         &idleUntilLocal,
				InstanceType:      "",
				MachineSize:       "S",
				UserTerminalKeyID: userKey.ID,
			}
			require.NoError(t, db.Create(local).Error)

			svc := services.NewTerminalTrainerService(db)
			resp, err := svc.SyncUserSessions(userID)
			require.NoError(t, err)

			// Find the per-session result — Updated must be false.
			var found bool
			for _, r := range resp.SessionResults {
				if r.SessionID == sessionID {
					found = true
					assert.False(t, r.Updated,
						"already-stopped %s row with expires_at == idle_until must NOT trigger an update (helper must be idempotent)", tc.mode)
				}
			}
			require.True(t, found, "session result for %s must be present in sync response", sessionID)
		})
	}
}

// ---------------------------------------------------------------------------
// (e) Running: helper does NOT fire — ExpiresAt untouched. Both modes.
// ---------------------------------------------------------------------------

func TestSyncUserSessions_Running_NotTouched(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	cases := []struct {
		name string
		mode string
	}{
		{"persistent", "persistent"},
		{"ephemeral", "ephemeral"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := "sync-running-" + tc.mode + "-" + uuid.New().String()
			idleUntil := time.Now().Add(30 * time.Minute).Truncate(time.Second)
			originalExpiry := time.Now().Add(45 * time.Minute).Truncate(time.Second)

			srv := syncSessionTTServer(t, sessionID,
				"running", tc.mode,
				originalExpiry.Unix(),
				idleUntil.Unix(), // even if tt-backend echoes idle_until on a running row, helper must skip
			)
			defer srv.Close()
			configureTTServer(t, srv.URL)

			db := freshTestDB(t)
			userID := "owner-running-" + tc.mode + "-" + uuid.New().String()
			userKey, err := createTestUserKey(db, userID)
			require.NoError(t, err)

			local := &models.Terminal{
				SessionID:         sessionID,
				UserID:            userID,
				Name:              "Test Terminal",
				State:             models.StateRunning,
				PersistenceMode:   tc.mode,
				ExpiresAt:         originalExpiry,
				InstanceType:      "",
				MachineSize:       "S",
				UserTerminalKeyID: userKey.ID,
			}
			require.NoError(t, db.Create(local).Error)

			svc := services.NewTerminalTrainerService(db)
			_, err = svc.SyncUserSessions(userID)
			require.NoError(t, err)

			var reloaded models.Terminal
			require.NoError(t, db.Where("session_id = ?", sessionID).First(&reloaded).Error)
			assert.Equal(t, models.StateRunning, reloaded.State, "running rows must stay running")

			delta := reloaded.ExpiresAt.Sub(originalExpiry)
			if delta.Abs() > time.Second {
				t.Errorf("running %s row must keep its original ExpiresAt (want ~%v, got %v) — markSessionStopped must only fire on state=stopped",
					tc.mode, originalExpiry, reloaded.ExpiresAt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// (f) Helper no-data branch via the public path: stopped row with no
//     SubscriptionPlanID and tt-backend omits idle_until → the only value
//     we can trust (the existing ExpiresAt) is kept untouched. Both modes.
// ---------------------------------------------------------------------------

func TestSyncUserSessions_Stopped_NoPlanNoIdleUntil_LeavesExpiresAtUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	cases := []struct {
		name string
		mode string
	}{
		{"persistent", "persistent"},
		{"ephemeral", "ephemeral"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := "sync-helper-noop-" + tc.mode + "-" + uuid.New().String()
			originalExpiry := time.Now().Add(2 * time.Hour).Truncate(time.Second)

			srv := syncSessionTTServer(t, sessionID,
				"stopped", tc.mode,
				originalExpiry.Unix(),
				0, // omit idle_until — helper must hit the no-data branch
			)
			defer srv.Close()
			configureTTServer(t, srv.URL)

			db := freshTestDB(t)
			userID := "owner-helper-noop-" + tc.mode + "-" + uuid.New().String()
			userKey, err := createTestUserKey(db, userID)
			require.NoError(t, err)

			// No SubscriptionPlanID — the plan fallback can't kick in either.
			local := &models.Terminal{
				SessionID:         sessionID,
				UserID:            userID,
				Name:              "Test Terminal",
				State:             models.StateRunning,
				PersistenceMode:   tc.mode,
				ExpiresAt:         originalExpiry,
				InstanceType:      "",
				MachineSize:       "S",
				UserTerminalKeyID: userKey.ID,
			}
			require.NoError(t, db.Create(local).Error)

			svc := services.NewTerminalTrainerService(db)
			_, err = svc.SyncUserSessions(userID)
			require.NoError(t, err)

			var reloaded models.Terminal
			require.NoError(t, db.Where("session_id = ?", sessionID).First(&reloaded).Error)
			assert.Equal(t, models.StateStopped, reloaded.State, "state must still propagate")
			delta := reloaded.ExpiresAt.Sub(originalExpiry)
			if delta.Abs() > time.Second {
				t.Errorf("with no idle_until and no plan, helper must leave ExpiresAt untouched (want ~%v, got %v)",
					originalExpiry, reloaded.ExpiresAt)
			}
		})
	}
}
