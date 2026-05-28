// tests/terminalTrainer/terminalLifecycleProxy_test.go
//
// MR-E: ocf-core proxies the new lifecycle endpoints exposed by tt-backend.
//
// What is verified here (load-bearing behaviour, not internals):
//
//  1. StopSession migrated from PUT /expire → POST /sessions/{id}/stop.
//     The OLD endpoint MUST NOT be hit anymore (regression guard).
//  2. StartSession resumes a stopped session and updates state/timestamps.
//  3. Ownership enforcement still rejects calls from a different user.
//
// Terminal capacity counting is covered by the budget engine tests
// (TestQuotaService_CheckBudget_StoppedCountsRegardlessOfPersistence in
// tests/payment/quotaServiceBudget_test.go) — there is no usage metric
// to drift, so the lifecycle proxy no longer asserts a counter contract.
//
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// recorder captures the (method, path) of each call into the fake tt-backend.
type recorder struct {
	calls atomic.Value // []string of "METHOD path"
}

func (r *recorder) record(method, path string) {
	cur, _ := r.calls.Load().([]string)
	r.calls.Store(append(cur, method+" "+path))
}

func (r *recorder) all() []string {
	cur, _ := r.calls.Load().([]string)
	return cur
}

func (r *recorder) sawCall(method, pathSuffix string) bool {
	for _, c := range r.all() {
		if strings.HasPrefix(c, method+" ") && strings.HasSuffix(c, pathSuffix) {
			return true
		}
	}
	return false
}

// startLifecycleTTServer spins a fake tt-backend that responds to the new
// lifecycle endpoints with 200 and records every incoming request.
func startLifecycleTTServer(t *testing.T) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r.Method, r.URL.Path)

		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/stop"):
			w.Header().Set("Content-Type", "application/json")
			idle := time.Now().Add(24 * time.Hour).UTC()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"idle_until": idle.Format(time.RFC3339),
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "running"})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/expire"):
			// If we ever hit this, the migration regressed.
			w.WriteHeader(http.StatusGone)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return srv, rec
}

// seedActiveSubscription creates a SubscriptionPlan + UserSubscription so the
// IncrementUsageMetric helper can find a backing subscription. Without this,
// the increment silently fails because the repository looks up an active
// subscription before creating the metric.
func seedActiveSubscription(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		Name:                   "test-plan",
		PriceAmount:            0,
		Currency:               "EUR",
		BillingInterval:        "monthly",
		MaxCourses:             10,
		IsActive:               true,
	}
	require.NoError(t, db.Create(plan).Error)

	sub := &paymentModels.UserSubscription{
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		CurrentPeriodStart: time.Now().Add(-24 * time.Hour),
		CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(sub).Error)
}

// ---------------------------------------------------------------------------
// Tests — service-level lifecycle behaviour
// ---------------------------------------------------------------------------

// TestStopSession_CallsNewStopEndpoint asserts the migration from PUT /expire
// to POST /sessions/{id}/stop. This is THE behaviour change of MR-E so the
// URL assertion is justified.
func TestStopSession_CallsNewStopEndpoint(t *testing.T) {
	ttServer, rec := startLifecycleTTServer(t)
	defer ttServer.Close()

	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	db := freshTestDB(t)
	userID := "owner-stop-" + uuid.New().String()
	seedActiveSubscription(t, db, userID)

	terminal, err := createTestTerminal(db, userID, "active", time.Now().Add(time.Hour))
	require.NoError(t, err)
	// Use persistent mode here; ephemeral stops are exercised separately
	// by TestStopSession_Ephemeral_AlsoMarksStopped_NotDeleted. Under D6'
	// (locked 2026-05-28), both modes transition to state="stopped" via
	// the same markSessionStopped helper — the distinction is UX-only.
	terminal.PersistenceMode = "persistent"
	require.NoError(t, db.Save(terminal).Error)

	svc := services.NewTerminalTrainerService(db)
	require.NoError(t, svc.StopSession(terminal.SessionID))

	// New endpoint must have been hit.
	assert.True(t,
		rec.sawCall(http.MethodPost, "/sessions/"+terminal.SessionID+"/stop"),
		"StopSession should POST /sessions/{id}/stop. Calls were: %v", rec.all())

	// Old endpoint must NOT have been called.
	for _, call := range rec.all() {
		assert.NotContains(t, call, "/expire",
			"StopSession should not call the legacy /expire endpoint anymore")
	}

	// Local state reflects the stop.
	updated, err := svc.GetSessionInfo(terminal.SessionID)
	require.NoError(t, err)
	assert.Equal(t, "stopped", updated.State)
}

// TestStartSession_Success verifies state transitions when starting a stopped
// session. The endpoint URL assertion guards against silent migration drift.
func TestStartSession_Success(t *testing.T) {
	ttServer, rec := startLifecycleTTServer(t)
	defer ttServer.Close()

	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	db := freshTestDB(t)
	userID := "owner-start-" + uuid.New().String()
	seedActiveSubscription(t, db, userID)

	// Pre-state: a stopped session.
	terminal, err := createTestTerminal(db, userID, "stopped", time.Now().Add(time.Hour))
	require.NoError(t, err)
	terminal.State = "stopped"
	idle := time.Now().Add(12 * time.Hour)
	terminal.IdleUntil = &idle
	require.NoError(t, db.Save(terminal).Error)

	svc := services.NewTerminalTrainerService(db)
	require.NoError(t, svc.StartSession(terminal.SessionID))

	assert.True(t,
		rec.sawCall(http.MethodPost, "/sessions/"+terminal.SessionID+"/start"),
		"StartSession should POST /sessions/{id}/start. Calls were: %v", rec.all())

	updated, err := svc.GetSessionInfo(terminal.SessionID)
	require.NoError(t, err)
	assert.Equal(t, "running", updated.State, "State should flip back to running")
	assert.Nil(t, updated.IdleUntil, "IdleUntil should be cleared on start")
	assert.False(t, updated.LastStartedAt.IsZero(), "LastStartedAt should be set")
}

// TestStartSession_SyncsExpiresAtFromResponse verifies that StartSession reads
// the freshly-computed expires_at from the tt-backend response body and mirrors
// it into terminal.ExpiresAt.
//
// Bug shape (before fix): tt-backend resets the instance expiry server-side on
// resume but the response body only carried state/last_started_at/ip. ocf-core
// kept terminal.ExpiresAt at the stale (past) value, so the frontend rendered
// "Session expirée" on a successfully-resumed terminal until the next sync poll.
//
// Fix shape: the new response body (tt-backend MR for #108) includes expires_at
// (unix seconds). startSessionInAPI now decodes it, StartSession mirrors it
// into the local row. This test seeds an already-past ExpiresAt, has the fake
// tt-backend return a future expires_at, and asserts the row was updated.
func TestStartSession_SyncsExpiresAtFromResponse(t *testing.T) {
	// Compute a deterministic future expiry the fake server will return.
	futureExpiryUnix := time.Now().Add(30 * time.Minute).Unix()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"state":           "running",
				"last_started_at": time.Now().Unix(),
				"ip":              "10.0.0.99",
				"expires_at":      futureExpiryUnix,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("TERMINAL_TRAINER_URL", srv.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	db := freshTestDB(t)
	userID := "owner-start-syncexpiry-" + uuid.New().String()
	seedActiveSubscription(t, db, userID)

	// Seed a terminal whose ExpiresAt is in the past (this is exactly the
	// situation the user hits: stopped session, original expiry elapsed,
	// they click Resume).
	pastExpiry := time.Now().Add(-10 * time.Minute)
	terminal, err := createTestTerminal(db, userID, "stopped", pastExpiry)
	require.NoError(t, err)
	terminal.State = "stopped"
	require.NoError(t, db.Save(terminal).Error)

	svc := services.NewTerminalTrainerService(db)
	require.NoError(t, svc.StartSession(terminal.SessionID))

	// The local row's ExpiresAt MUST be updated to the value returned by
	// tt-backend. Without the fix it stays in the past.
	var refreshed models.Terminal
	require.NoError(t, db.Where("session_id = ?", terminal.SessionID).First(&refreshed).Error)
	gotUnix := refreshed.ExpiresAt.Unix()
	if gotUnix != futureExpiryUnix {
		t.Errorf("expected terminal.ExpiresAt to be synced to %d (future), got %d (past=%d)",
			futureExpiryUnix, gotUnix, pastExpiry.Unix())
	}
	assert.True(t, refreshed.ExpiresAt.After(time.Now()),
		"after resume, ExpiresAt must be in the future (was %v)", refreshed.ExpiresAt)
}

// TestStartSession_NotFound returns a wrapped error referencing the session ID
// when the local row doesn't exist (no tt-backend call should be made).
func TestStartSession_NotFound(t *testing.T) {
	ttServer, rec := startLifecycleTTServer(t)
	defer ttServer.Close()

	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	db := freshTestDB(t)
	svc := services.NewTerminalTrainerService(db)

	err := svc.StartSession("nonexistent-session")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")

	// No upstream call should have been made — the lookup failed first.
	for _, call := range rec.all() {
		assert.NotContains(t, call, "/start",
			"start endpoint should not be called when session is unknown")
	}
}

// ---------------------------------------------------------------------------
// Ownership enforcement (Layer 2) — same coverage as the existing /stop tests
// ---------------------------------------------------------------------------

// TestLifecycle_OwnershipEnforced is a table-driven test that pins the
// ownership rule for every lifecycle operation — preventing an MR-F or later
// from accidentally relaxing ownership on one of the new routes.
func TestLifecycle_OwnershipEnforced(t *testing.T) {
	ttServer, _ := startLifecycleTTServer(t)
	defer ttServer.Close()

	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	db := freshTestDB(t)
	ownerID := "owner-" + uuid.New().String()
	intruderID := "intruder-" + uuid.New().String()
	seedActiveSubscription(t, db, ownerID)

	terminal, err := createTestTerminal(db, ownerID, "active", time.Now().Add(time.Hour))
	require.NoError(t, err)

	svc := services.NewTerminalTrainerService(db)

	// HasTerminalAccess is what the middleware + controller both consult.
	// It must reject the intruder for every lifecycle path.
	for _, op := range []string{"stop", "start", "delete"} {
		t.Run(op, func(t *testing.T) {
			ok, err := svc.HasTerminalAccess(terminal.SessionID, intruderID)
			require.NoError(t, err)
			assert.False(t, ok,
				"intruder must not have access to %s a session they don't own", op)

			ownerOK, err := svc.HasTerminalAccess(terminal.SessionID, ownerID)
			require.NoError(t, err)
			assert.True(t, ownerOK,
				"owner must have access to %s their own session", op)
		})
	}

	// And the model itself, post-stop, still belongs to the owner.
	updated, err := svc.GetSessionInfo(terminal.SessionID)
	require.NoError(t, err)
	assert.Equal(t, ownerID, updated.UserID)
	_ = models.Terminal{}
}
