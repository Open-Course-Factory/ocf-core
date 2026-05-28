// tests/terminalTrainer/syncStoppedPersistentExpiry_test.go
//
// Follow-up to commit ae8aba7 (StopSession + persistent expires_at extension):
// user-initiated stops were fixed, but tt-backend auto-stops reach the local
// row through SyncUserSessions, which propagates state and idle_until but did
// NOT touch expires_at. The original creation deadline was already in the past
// (that's precisely why tt-backend auto-stopped), so BudgetOccupyingScope
// dropped the row — /Utilisation Actuelle showed empty bars and the budget
// gate let the user launch new sessions even though resumable persistent rows
// were still on the books.
//
// These tests pin the contract for the sync path: whenever SyncUserSessions
// transitions (or maintains) a row in state="stopped" + persistence="persistent",
// the helper applyStoppedPersistentExpiry must extend expires_at to the
// tt-backend reap deadline (idle_until), with a plan-derived fallback when
// idle_until is missing.
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
// (a) Auto-stopped persistent: sync must extend ExpiresAt to idle_until.
// ---------------------------------------------------------------------------

func TestSyncUserSessions_AutoStoppedPersistent_ExtendsExpiresAtToIdleUntil(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	sessionID := "sync-auto-stop-" + uuid.New().String()
	idleUntil := time.Now().Add(30 * time.Minute).Truncate(time.Second)
	originalExpiry := time.Now().Add(-time.Minute) // already past

	srv := syncSessionTTServer(t, sessionID,
		"stopped", "persistent",
		originalExpiry.Unix(), // tt-backend echoes its own (past) expiry
		idleUntil.Unix(),
	)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-auto-stop-" + uuid.New().String()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Local row: still in "running" with the original creation deadline that
	// has just elapsed. This is exactly the state ocf-core was in when
	// tt-backend reaped the session.
	local := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Test Terminal",
		State:             "running",
		PersistenceMode:   "persistent",
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
	assert.Equal(t, "stopped", reloaded.State,
		"sync must propagate tt-backend's stopped state")
	assert.Equal(t, "persistent", reloaded.PersistenceMode)

	// ExpiresAt must be aligned with idle_until (tolerate 1s clock drift).
	delta := reloaded.ExpiresAt.Sub(idleUntil)
	if delta < -time.Second || delta > time.Second {
		t.Errorf("expected ExpiresAt ~= idleUntil (%v), got %v (delta=%v) — sync must extend expires_at on stopped persistent rows so BudgetOccupyingScope keeps counting them",
			idleUntil, reloaded.ExpiresAt, delta)
	}
	assert.True(t, reloaded.ExpiresAt.After(time.Now()),
		"after sync, stopped-persistent ExpiresAt must be in the future")
}

// ---------------------------------------------------------------------------
// (b) Auto-stopped persistent, idle_until absent: falls back to plan duration.
// ---------------------------------------------------------------------------

func TestSyncUserSessions_AutoStoppedPersistent_IdleUntilZero_UsesPlanFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	sessionID := "sync-auto-stop-no-idle-" + uuid.New().String()
	originalExpiry := time.Now().Add(-time.Minute)

	srv := syncSessionTTServer(t, sessionID,
		"stopped", "persistent",
		originalExpiry.Unix(),
		0, // omit idle_until — exercise the plan-derived fallback
	)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-auto-stop-fallback-" + uuid.New().String()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	local := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Test Terminal",
		State:             "running",
		PersistenceMode:   "persistent",
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
	assert.Equal(t, "stopped", reloaded.State)

	expectedLow := before.Add(60 * time.Minute).Add(-time.Second)
	expectedHigh := after.Add(60 * time.Minute).Add(5 * time.Second)
	if reloaded.ExpiresAt.Before(expectedLow) || reloaded.ExpiresAt.After(expectedHigh) {
		t.Errorf("expected ExpiresAt in [%v, %v], got %v — sync must fall back to plan.MaxSessionDurationMinutes when tt-backend omits idle_until",
			expectedLow, expectedHigh, reloaded.ExpiresAt)
	}
}

// ---------------------------------------------------------------------------
// (c) Auto-stopped ephemeral: ExpiresAt unchanged (container is in tear-down).
// ---------------------------------------------------------------------------

func TestSyncUserSessions_AutoStoppedEphemeral_NoExpiresAtExtension(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	sessionID := "sync-auto-stop-ephemeral-" + uuid.New().String()
	idleUntil := time.Now().Add(30 * time.Minute).Truncate(time.Second)
	originalExpiry := time.Now().Add(15 * time.Minute) // still future, just to make
	// sure the assertion catches accidental extension regardless of original
	// being past or future.

	srv := syncSessionTTServer(t, sessionID,
		"stopped", "ephemeral",
		originalExpiry.Unix(),
		idleUntil.Unix(), // present, but must be ignored for ephemeral
	)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-auto-stop-ephemeral-" + uuid.New().String()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	local := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Test Terminal",
		State:             "running",
		PersistenceMode:   "ephemeral",
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
	assert.Equal(t, "ephemeral", reloaded.PersistenceMode,
		"sync must preserve ephemeral persistence mode")

	// ExpiresAt must NOT have been bumped to idle_until. Allow microsecond
	// drift from DB round-trip but anything bigger is a regression.
	delta := reloaded.ExpiresAt.Sub(originalExpiry)
	if delta.Abs() > time.Second {
		t.Errorf("ephemeral stop must leave ExpiresAt untouched (want ~%v, got %v) — the container is in tear-down, idle_until is meaningless",
			originalExpiry, reloaded.ExpiresAt)
	}
}

// ---------------------------------------------------------------------------
// (d) Already-stopped persistent with expires_at == idle_until: idempotent.
// ---------------------------------------------------------------------------

func TestSyncUserSessions_AlreadyStoppedPersistent_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	sessionID := "sync-idempotent-" + uuid.New().String()
	idleUntil := time.Now().Add(30 * time.Minute).Truncate(time.Second)

	srv := syncSessionTTServer(t, sessionID,
		"stopped", "persistent",
		idleUntil.Unix(), // tt-backend's expires_at echoes the same window
		idleUntil.Unix(),
	)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-idempotent-" + uuid.New().String()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Local row already in the post-fix steady state: stopped+persistent with
	// expires_at = idle_until. Sync must be a no-op (no spurious writes).
	idleUntilLocal := idleUntil.Local()
	local := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Test Terminal",
		State:             "stopped",
		PersistenceMode:   "persistent",
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
				"already-stopped persistent row with expires_at == idle_until must NOT trigger an update (helper must be idempotent)")
		}
	}
	require.True(t, found, "session result for %s must be present in sync response", sessionID)
}

// ---------------------------------------------------------------------------
// (e) Running persistent: helper does NOT fire — ExpiresAt untouched.
// ---------------------------------------------------------------------------

func TestSyncUserSessions_RunningPersistent_NotTouched(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	sessionID := "sync-running-" + uuid.New().String()
	idleUntil := time.Now().Add(30 * time.Minute).Truncate(time.Second)
	originalExpiry := time.Now().Add(45 * time.Minute).Truncate(time.Second)

	srv := syncSessionTTServer(t, sessionID,
		"running", "persistent",
		originalExpiry.Unix(),
		idleUntil.Unix(), // even if tt-backend echoes idle_until on a running row, helper must skip
	)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-running-" + uuid.New().String()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	local := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Test Terminal",
		State:             "running",
		PersistenceMode:   "persistent",
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
	assert.Equal(t, "running", reloaded.State, "running rows must stay running")

	delta := reloaded.ExpiresAt.Sub(originalExpiry)
	if delta.Abs() > time.Second {
		t.Errorf("running persistent row must keep its original ExpiresAt (want ~%v, got %v) — helper must only fire on state=stopped",
			originalExpiry, reloaded.ExpiresAt)
	}
}

// ---------------------------------------------------------------------------
// (f) Helper no-data branch via the public path: stopped persistent row with
//     no SubscriptionPlanID and tt-backend omits idle_until → the only value
//     we can trust (the existing ExpiresAt) is kept untouched.
// ---------------------------------------------------------------------------

func TestSyncUserSessions_StoppedPersistent_NoPlanNoIdleUntil_LeavesExpiresAtUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	sessionID := "sync-helper-noop-" + uuid.New().String()
	originalExpiry := time.Now().Add(2 * time.Hour).Truncate(time.Second)

	srv := syncSessionTTServer(t, sessionID,
		"stopped", "persistent",
		originalExpiry.Unix(),
		0, // omit idle_until — helper must hit the no-data branch
	)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-helper-noop-" + uuid.New().String()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// No SubscriptionPlanID — the plan fallback can't kick in either.
	local := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Test Terminal",
		State:             "running",
		PersistenceMode:   "persistent",
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
	assert.Equal(t, "stopped", reloaded.State, "state must still propagate")
	delta := reloaded.ExpiresAt.Sub(originalExpiry)
	if delta.Abs() > time.Second {
		t.Errorf("with no idle_until and no plan, helper must leave ExpiresAt untouched (want ~%v, got %v)",
			originalExpiry, reloaded.ExpiresAt)
	}
}
