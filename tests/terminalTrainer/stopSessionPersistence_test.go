// tests/terminalTrainer/stopSessionPersistence_test.go
//
// These tests pin the StopSession + SyncUserSessions contract under rule
// D6' (locked 2026-05-28, supersedes D6): "a stop is a stop". Every
// stopped session — persistent or ephemeral — counts against the budget
// and reserves its slot until tt-backend confirms the container is gone.
//
//   - stop: state="stopped" and expires_at is extended to the tt-backend
//     reap deadline (idle_until, with plan-derived fallback). One unified
//     path via markSessionStopped — no persistence_mode branch.
//
//   - sync: tt-backend is SSOT for container existence — if a local row
//     is missing from the API, it must be marked deleted regardless of
//     its current state. While it is still listed by tt-backend, sync
//     mirrors the API state through the same markSessionStopped helper.
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
// (a) Stop extends expires_at to the idle_until from tt-backend — both
//     persistence modes. The contract is now mode-independent.
// ---------------------------------------------------------------------------

func TestStopSession_ExtendsExpiresAtToIdleUntil(t *testing.T) {
	cases := []struct {
		name string
		mode string
	}{
		{"persistent", "persistent"},
		{"ephemeral", "ephemeral"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idleUntil := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
			srv := stopOnlyTTServer(t, idleUntil.Format(time.RFC3339))
			defer srv.Close()
			configureTTServer(t, srv.URL)

			db := freshTestDB(t)
			userID := "owner-stop-" + tc.mode + "-" + uuid.New().String()
			seedActiveSubscription(t, db, userID)

			// Original expiry near-now — this is the regression case: after stop,
			// the original deadline elapses but the row should stay budget-visible.
			originalExpiry := time.Now().Add(2 * time.Minute)
			terminal, err := createTestTerminal(db, userID, "running", originalExpiry)
			require.NoError(t, err)
			terminal.PersistenceMode = tc.mode
			require.NoError(t, db.Save(terminal).Error)

			svc := services.NewTerminalTrainerService(db)
			require.NoError(t, svc.StopSession(terminal.SessionID))

			var reloaded models.Terminal
			require.NoError(t, db.Where("session_id = ?", terminal.SessionID).First(&reloaded).Error)
			assert.Equal(t, "stopped", reloaded.State,
				"a stop is a stop: %s stop must mark state=stopped (D6')", tc.mode)

			// ExpiresAt must be aligned with idle_until (tolerate 1s clock drift /
			// JSON round-trip truncation).
			delta := reloaded.ExpiresAt.Sub(idleUntil)
			if delta < -time.Second || delta > time.Second {
				t.Errorf("expected ExpiresAt ~= idleUntil (%v), got %v (delta=%v)",
					idleUntil, reloaded.ExpiresAt, delta)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// (b) Stop with idle_until nil falls back to plan duration — both modes.
// ---------------------------------------------------------------------------

func TestStopSession_IdleUntilNil_FallsBackToPlanDuration(t *testing.T) {
	cases := []struct {
		name string
		mode string
	}{
		{"persistent", "persistent"},
		{"ephemeral", "ephemeral"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := stopOnlyTTServer(t, "") // tt-backend returns no idle_until
			defer srv.Close()
			configureTTServer(t, srv.URL)

			db := freshTestDB(t)
			userID := "owner-stop-fallback-" + tc.mode + "-" + uuid.New().String()
			seedActiveSubscription(t, db, userID)

			originalExpiry := time.Now().Add(2 * time.Minute)
			terminal, err := createTestTerminal(db, userID, "running", originalExpiry)
			require.NoError(t, err)
			terminal.PersistenceMode = tc.mode
			require.NoError(t, db.Save(terminal).Error)

			// Plan dictates the fallback: 60 minutes.
			seedPlanForTerminal(t, db, terminal, 60)

			before := time.Now()
			svc := services.NewTerminalTrainerService(db)
			require.NoError(t, svc.StopSession(terminal.SessionID))
			after := time.Now()

			var reloaded models.Terminal
			require.NoError(t, db.Where("session_id = ?", terminal.SessionID).First(&reloaded).Error)
			assert.Equal(t, "stopped", reloaded.State,
				"a stop is a stop: %s stop must mark state=stopped (D6')", tc.mode)

			// ExpiresAt should be within [before+60min, after+60min+5s tolerance].
			expectedLow := before.Add(60 * time.Minute).Add(-time.Second)
			expectedHigh := after.Add(60 * time.Minute).Add(5 * time.Second)
			if reloaded.ExpiresAt.Before(expectedLow) || reloaded.ExpiresAt.After(expectedHigh) {
				t.Errorf("expected ExpiresAt in [%v, %v], got %v",
					expectedLow, expectedHigh, reloaded.ExpiresAt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// (c) Ephemeral stop ALSO marks state="stopped" (NOT "deleted"). This is the
//     load-bearing change of D6': ephemeral stops go through markSessionStopped
//     just like persistent stops. The local row stays stopped until sync
//     observes tt-backend has reaped the container, at which point step 5b
//     marks it deleted. This pins that the new rule actually fires.
// ---------------------------------------------------------------------------

func TestStopSession_Ephemeral_AlsoMarksStopped_NotDeleted(t *testing.T) {
	idleUntil := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	srv := stopOnlyTTServer(t, idleUntil.Format(time.RFC3339))
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "owner-stop-ephemeral-not-deleted-" + uuid.New().String()
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
	assert.Equal(t, "stopped", reloaded.State,
		"ephemeral stop must NOT mark deleted anymore (D6'): the slot stays reserved until sync confirms tt-backend reaped the container")
	assert.Equal(t, "ephemeral", reloaded.PersistenceMode,
		"PersistenceMode must be preserved across stop")
}

// ---------------------------------------------------------------------------
// (d) End-to-end: a stopped session — regardless of persistence mode —
//     stays in budget after the original expiry, until sync confirms the
//     container is gone. This is the user's pinned scenario.
// ---------------------------------------------------------------------------

func TestStopSession_StillCountedInBudgetAfterOriginalExpiry(t *testing.T) {
	cases := []struct {
		name string
		mode string
	}{
		{"persistent", "persistent"},
		{"ephemeral", "ephemeral"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// idle_until 30 minutes in the future, well past the original expiry.
			idleUntil := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
			srv := stopOnlyTTServer(t, idleUntil.Format(time.RFC3339))
			defer srv.Close()
			configureTTServer(t, srv.URL)

			db := freshTestDB(t)
			userID := "owner-budget-after-stop-" + tc.mode + "-" + uuid.New().String()
			seedActiveSubscription(t, db, userID)

			// Original expiry already past — simulates the user's reported scenario:
			// they stop a session whose creation-time + plan duration has just elapsed
			// (clock is barely past). Before D6', OccupiesSlotScope still kept the row
			// for persistent — the old BudgetOccupyingScope would also drop it on
			// ephemeral. D6' unifies: both modes stay counted.
			terminal, err := createTestTerminal(db, userID, "running", time.Now().Add(-time.Minute))
			require.NoError(t, err)
			terminal.PersistenceMode = tc.mode
			terminal.SizeCPU = 2
			terminal.SizeMemoryMB = 2048
			require.NoError(t, db.Save(terminal).Error)

			svc := services.NewTerminalTrainerService(db)
			require.NoError(t, svc.StopSession(terminal.SessionID))

			// Sanity: row is now stopped with the extended expires_at.
			var reloaded models.Terminal
			require.NoError(t, db.Where("session_id = ?", terminal.SessionID).First(&reloaded).Error)
			require.Equal(t, "stopped", reloaded.State)
			require.Equal(t, tc.mode, reloaded.PersistenceMode,
				"PersistenceMode must be preserved across stop")
			require.Equal(t, 2, reloaded.SizeCPU, "SizeCPU must survive stop")
			require.Equal(t, 2048, reloaded.SizeMemoryMB, "SizeMemoryMB must survive stop")
			require.True(t, reloaded.ExpiresAt.After(time.Now()),
				"after stop, ExpiresAt must be in the future, got %v", reloaded.ExpiresAt)

			// Budget panel: GetBudgetUsage must include the stopped session.
			qs := paymentServices.NewQuotaService(db, nil)
			usedCPU, usedMem, err := qs.GetBudgetUsage(userID, nil)
			require.NoError(t, err)
			assert.Equal(t, 2, usedCPU,
				"stopped %s terminal must still count CPU in budget (D6')", tc.mode)
			assert.Equal(t, 2048, usedMem,
				"stopped %s terminal must still count RAM in budget (D6')", tc.mode)
		})
	}
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
