// tests/terminalTrainer/stopSessionPersistence_test.go
//
// These tests pin the StopSession + SyncUserSessions contract for the
// "stopped persistent vs ephemeral" lifecycle bug:
//
//   - persistent stop: extend expires_at so BudgetOccupyingScope keeps the
//     row visible until tt-backend would actually reap it. Without this
//     the budget panel and the gate stop counting capacity that the user
//     can still resume from /terminal-sessions.
//
//   - ephemeral stop: the container is destroyed by tt-backend, the local
//     row must be marked deleted (not stopped) so it stops appearing as a
//     ghost in the sessions list.
//
//   - sync: tt-backend is SSOT for container existence — if a local row is
//     missing from the API, it must be marked deleted regardless of
//     whether its current state is "stopped" or anything else.
//
// All scenarios use the real terminalTrainerService against an in-memory
// SQLite DB and a fake tt-backend HTTP server (matching the convention of
// terminalLifecycleProxy_test.go and sessionStatePropagation_test.go).
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

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"

	"gorm.io/gorm"
)

// stopOnlyTTServer responds to POST /sessions/{id}/stop with the supplied
// idle_until (RFC3339) — set to empty to omit the field entirely. Anything
// else returns 404 so unexpected calls fail loudly.
func stopOnlyTTServer(t *testing.T, idleUntilRFC3339 string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/stop") {
			w.Header().Set("Content-Type", "application/json")
			if idleUntilRFC3339 == "" {
				_, _ = w.Write([]byte(`{}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"idle_until": idleUntilRFC3339,
			})
			return
		}
		http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}))
}

// emptySessionsListTTServer responds to GET /1.0/sessions with an empty list
// AND to POST /stop (in case it's called) — used by the SyncUserSessions
// tests that need tt-backend to "not list" a particular local row.
func emptySessionsListTTServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/1.0/sessions" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions":        []any{},
				"count":           0,
				"api_key_id":      0,
				"include_expired": true,
				"limit":           1000,
			})
			return
		}
		http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}))
}

// sessionListContainingTTServer returns a /sessions response that includes
// the given session (so SyncUserSessions sees the local row mirrored in the API).
// It accepts both /1.0/sessions and /1.0/{instance_type}/sessions paths
// because getAllSessionsFromAllInstanceTypes iterates every type seen locally.
func sessionListContainingTTServer(t *testing.T, sessionID, apiState string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sessions") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions": []map[string]any{
					{
						"id":               sessionID,
						"session_id":       sessionID,
						"name":             "tst",
						"status":           1, // SessionStatusExpired in tt-backend's enum
						"expires_at":       time.Now().Add(time.Hour).Unix(),
						"created_at":       time.Now().Add(-time.Hour).Unix(),
						"state":            apiState,
						"persistence_mode": "persistent",
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

// seedPlanForTerminal creates a SubscriptionPlan with the given
// MaxSessionDurationMinutes and attaches it to the terminal row. Returns
// the plan ID so tests can refer back to it.
func seedPlanForTerminal(t *testing.T, db *gorm.DB, terminal *models.Terminal, maxDurationMin int) uuid.UUID {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:                 entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                      "stop-test-plan-" + uuid.New().String(),
		PriceAmount:               0,
		Currency:                  "EUR",
		BillingInterval:           "monthly",
		MaxCourses:                10,
		MaxSessionDurationMinutes: maxDurationMin,
		IsActive:                  true,
	}
	require.NoError(t, db.Create(plan).Error)
	terminal.SubscriptionPlanID = &plan.ID
	require.NoError(t, db.Save(terminal).Error)
	return plan.ID
}

// ---------------------------------------------------------------------------
// (a) Persistent stop extends expires_at to the idle_until from tt-backend.
// ---------------------------------------------------------------------------

func TestStopSession_Persistent_ExtendsExpiresAtToIdleUntil(t *testing.T) {
	idleUntil := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	srv := stopOnlyTTServer(t, idleUntil.Format(time.RFC3339))
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-stop-persistent-" + uuid.New().String()
	seedActiveSubscription(t, db, userID)

	// Original expiry near-now — this is the regression case: after stop,
	// the original deadline elapses but the row should stay budget-visible.
	originalExpiry := time.Now().Add(2 * time.Minute)
	terminal, err := createTestTerminal(db, userID, "running", originalExpiry)
	require.NoError(t, err)
	terminal.PersistenceMode = "persistent"
	require.NoError(t, db.Save(terminal).Error)

	svc := services.NewTerminalTrainerService(db)
	require.NoError(t, svc.StopSession(terminal.SessionID))

	var reloaded models.Terminal
	require.NoError(t, db.Where("session_id = ?", terminal.SessionID).First(&reloaded).Error)
	assert.Equal(t, "stopped", reloaded.State, "persistent stop must mark state=stopped")

	// ExpiresAt must be aligned with idle_until (tolerate 1s clock drift /
	// JSON round-trip truncation).
	delta := reloaded.ExpiresAt.Sub(idleUntil)
	if delta < -time.Second || delta > time.Second {
		t.Errorf("expected ExpiresAt ~= idleUntil (%v), got %v (delta=%v)",
			idleUntil, reloaded.ExpiresAt, delta)
	}
}

// ---------------------------------------------------------------------------
// (b) Persistent stop with idle_until nil falls back to plan duration.
// ---------------------------------------------------------------------------

func TestStopSession_Persistent_IdleUntilNil_FallsBackToPlanDuration(t *testing.T) {
	srv := stopOnlyTTServer(t, "") // tt-backend returns no idle_until
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-stop-persistent-fallback-" + uuid.New().String()
	seedActiveSubscription(t, db, userID)

	originalExpiry := time.Now().Add(2 * time.Minute)
	terminal, err := createTestTerminal(db, userID, "running", originalExpiry)
	require.NoError(t, err)
	terminal.PersistenceMode = "persistent"
	require.NoError(t, db.Save(terminal).Error)

	// Plan dictates the fallback: 60 minutes.
	seedPlanForTerminal(t, db, terminal, 60)

	before := time.Now()
	svc := services.NewTerminalTrainerService(db)
	require.NoError(t, svc.StopSession(terminal.SessionID))
	after := time.Now()

	var reloaded models.Terminal
	require.NoError(t, db.Where("session_id = ?", terminal.SessionID).First(&reloaded).Error)
	assert.Equal(t, "stopped", reloaded.State, "persistent stop must mark state=stopped")

	// ExpiresAt should be within [before+60min, after+60min+5s tolerance].
	expectedLow := before.Add(60 * time.Minute).Add(-time.Second)
	expectedHigh := after.Add(60 * time.Minute).Add(5 * time.Second)
	if reloaded.ExpiresAt.Before(expectedLow) || reloaded.ExpiresAt.After(expectedHigh) {
		t.Errorf("expected ExpiresAt in [%v, %v], got %v",
			expectedLow, expectedHigh, reloaded.ExpiresAt)
	}
}

// ---------------------------------------------------------------------------
// (c) Ephemeral stop marks the row deleted (container destroyed by tt-backend).
// ---------------------------------------------------------------------------

func TestStopSession_Ephemeral_MarksDeleted(t *testing.T) {
	// Send an idle_until anyway — it must be ignored for ephemeral.
	idleUntil := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	srv := stopOnlyTTServer(t, idleUntil.Format(time.RFC3339))
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-stop-ephemeral-" + uuid.New().String()
	seedActiveSubscription(t, db, userID)

	originalExpiry := time.Now().Add(time.Hour)
	terminal, err := createTestTerminal(db, userID, "running", originalExpiry)
	require.NoError(t, err)
	terminal.PersistenceMode = "ephemeral"
	require.NoError(t, db.Save(terminal).Error)

	svc := services.NewTerminalTrainerService(db)
	require.NoError(t, svc.StopSession(terminal.SessionID))

	var reloaded models.Terminal
	require.NoError(t, db.Where("session_id = ?", terminal.SessionID).First(&reloaded).Error)
	assert.Equal(t, "deleted", reloaded.State,
		"ephemeral stop must mark state=deleted; tt-backend destroyed the container")
	// ExpiresAt must NOT be touched — the original deadline is the only
	// data we can trust about an already-destroyed container.
	if !reloaded.ExpiresAt.Equal(originalExpiry.Truncate(time.Microsecond)) &&
		reloaded.ExpiresAt.Sub(originalExpiry).Abs() > time.Second {
		t.Errorf("ephemeral stop must leave ExpiresAt untouched (want ~%v, got %v)",
			originalExpiry, reloaded.ExpiresAt)
	}
}

// ---------------------------------------------------------------------------
// (d) End-to-end: stopped persistent stays in budget after original expiry.
// ---------------------------------------------------------------------------

func TestStopSession_Persistent_StillCountedInBudgetAfterOriginalExpiry(t *testing.T) {
	// idle_until 30 minutes in the future, well past the original expiry.
	idleUntil := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	srv := stopOnlyTTServer(t, idleUntil.Format(time.RFC3339))
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-budget-after-stop-" + uuid.New().String()
	seedActiveSubscription(t, db, userID)

	// Original expiry already past — simulates the user's reported scenario:
	// they stop a session whose creation-time + plan duration has just elapsed
	// (clock is barely past). Before the fix, BudgetOccupyingScope drops it
	// the moment now() > original ExpiresAt.
	terminal, err := createTestTerminal(db, userID, "running", time.Now().Add(-time.Minute))
	require.NoError(t, err)
	terminal.PersistenceMode = "persistent"
	terminal.SizeCPU = 2
	terminal.SizeMemoryMB = 2048
	require.NoError(t, db.Save(terminal).Error)

	svc := services.NewTerminalTrainerService(db)
	require.NoError(t, svc.StopSession(terminal.SessionID))

	// Sanity: row is now stopped with the extended expires_at.
	var reloaded models.Terminal
	require.NoError(t, db.Where("session_id = ?", terminal.SessionID).First(&reloaded).Error)
	require.Equal(t, "stopped", reloaded.State)
	require.Equal(t, "persistent", reloaded.PersistenceMode,
		"PersistenceMode must be preserved across stop")
	require.Equal(t, 2, reloaded.SizeCPU, "SizeCPU must survive stop")
	require.Equal(t, 2048, reloaded.SizeMemoryMB, "SizeMemoryMB must survive stop")
	require.True(t, reloaded.ExpiresAt.After(time.Now()),
		"after persistent stop, ExpiresAt must be in the future, got %v", reloaded.ExpiresAt)

	// Budget panel: GetBudgetUsage must include the stopped persistent.
	qs := paymentServices.NewQuotaService(db, nil)
	usedCPU, usedMem, err := qs.GetBudgetUsage(userID, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, usedCPU, "stopped persistent terminal must still count CPU in budget")
	assert.Equal(t, 2048, usedMem, "stopped persistent terminal must still count RAM in budget")
}

// ---------------------------------------------------------------------------
// (e) SyncUserSessions: stopped row missing from API → marked deleted.
// ---------------------------------------------------------------------------

func TestSyncUserSessions_StoppedSession_MissingFromAPI_MarkedDeleted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	srv := emptySessionsListTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-sync-stopped-missing-" + uuid.New().String()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Local row in state=stopped, no matching entry in API list.
	local := &models.Terminal{
		SessionID:         "sync-stopped-missing-" + uuid.New().String(),
		UserID:            userID,
		Name:              "Test Terminal",
		State:             "stopped",
		PersistenceMode:   "persistent",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(local).Error)

	svc := services.NewTerminalTrainerService(db)
	_, err = svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var reloaded models.Terminal
	require.NoError(t, db.Where("session_id = ?", local.SessionID).First(&reloaded).Error)
	assert.Equal(t, "deleted", reloaded.State,
		"stopped local row missing from tt-backend must be marked deleted: tt-backend is SSOT for container existence")
}

// ---------------------------------------------------------------------------
// (f) SyncUserSessions: stopped row STILL present in API → stays stopped.
// ---------------------------------------------------------------------------

func TestSyncUserSessions_StoppedSession_StillInAPI_KeptStopped(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	sessionID := "sync-stopped-present-" + uuid.New().String()
	srv := sessionListContainingTTServer(t, sessionID, "stopped")
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-sync-stopped-present-" + uuid.New().String()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	local := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Test Terminal",
		State:             "stopped",
		PersistenceMode:   "persistent",
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(local).Error)

	svc := services.NewTerminalTrainerService(db)
	_, err = svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var reloaded models.Terminal
	require.NoError(t, db.Where("session_id = ?", local.SessionID).First(&reloaded).Error)
	assert.Equal(t, "stopped", reloaded.State,
		"stopped local row that tt-backend still acknowledges must stay stopped")
}
