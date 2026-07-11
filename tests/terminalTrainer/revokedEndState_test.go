// tests/terminalTrainer/revokedEndState_test.go
//
// RED tests for issue #388 (June payment go-live blocker #3): a distinct
// "revoked" end state for terminals killed by a billing lapse / plan
// revocation.
//
// Today the billing-cleanup path (payment/services.TerminateUserTerminals)
// marks a live learner's terminal State="deleted" — the SAME value a normal
// TTL expiry produces. The frontend end-state logic reads the `state` field
// from GET /terminals/user-sessions (SSOT: ocf-front src/utils/sessionState.ts
// + TerminalSessionView.vue:604, which maps state==='deleted' -> the "Session
// Expired — time limit" banner). A revoked learner is therefore told they ran
// out of time. The fix is a distinct State value "revoked", surfaced verbatim
// by the exact endpoint the frontend consumes.
//
// These tests pin the WIRE STRING "revoked" (not a Go constant) on purpose:
//   - the string is the cross-repo contract consumed by ocf-front;
//   - referencing a not-yet-existing models.StateRevoked constant would break
//     package compilation and hide the runtime RED. Green work should add
//     models.StateRevoked TerminalState = "revoked" and keep these strings.
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
)

// revokedWireState is the exact string the frontend discriminates on. Kept as a
// local literal so these tests compile before models.StateRevoked exists.
const revokedWireState = "revoked"

// TestGetUserSessions_RevokedSession_ExposesRevokedState is the primary,
// frontend-facing contract test. It drives the EXACT endpoint the frontend end-
// state logic reads (GET /terminals/user-sessions) and asserts the persisted
// "revoked" state round-trips onto the wire as `state`.
//
// The worst case is pinned deliberately: the terminal is revoked in ocf-core
// (State="revoked") but tt-backend STILL lists the session as active (Status 0)
// — which is realistic, because TerminateUserTerminals only updates the ocf-core
// DB and never calls tt-backend to tear down the container. GetUserSessions runs
// a pre-list SyncUserSessions that today clobbers any non-stopped local state
// back to the tt-backend truth (terminalSyncService.go:223), so a naive
// implementation would return "running" and the learner would silently regain a
// revoked session. The endpoint must instead preserve "revoked".
//
// RED today: pre-list sync overwrites "revoked" -> "running".
func TestGetUserSessions_RevokedSession_ExposesRevokedState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	userID := "revoked-endpoint-user"
	sessionID := "sess-revoked-endpoint"

	// tt-backend still reports the session as active — the revoked state exists
	// only in ocf-core and must not be reconciled away.
	sessions := []dto.TerminalTrainerSession{
		{
			SessionID:   sessionID,
			Status:      0, // active
			ExpiresAt:   time.Now().Add(1 * time.Hour).Unix(),
			State:       models.StateRunning,
			MachineSize: "S",
		},
	}
	srv := syncStubBackend(t, func() []dto.TerminalTrainerSession { return sessions })
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	controller := terminalController.NewTerminalController(db)

	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Seed a terminal already revoked by the billing-cleanup path.
	require.NoError(t, db.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Revoked Session",
		State:             models.TerminalState(revokedWireState),
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"user"})
		c.Next()
	})
	router.GET("/terminals/user-sessions", controller.GetUserSessions)

	req := httptest.NewRequest("GET", "/terminals/user-sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 1, "the revoked session must still be listed (isActive=false path)")

	assert.Equal(t, revokedWireState, rows[0]["state"],
		"GET /terminals/user-sessions must surface state=\"revoked\" so the frontend can show "+
			"honest revocation copy instead of the \"Session Expired — time limit\" banner")
}

// TestSyncUserSessions_RevokedRow_NotClobberedByActiveAPI isolates the
// reconciliation rule behind the endpoint test above: a revoked local row must
// survive a sync pass even while tt-backend still lists the session as active.
// Without protection, terminalSyncService overwrites "revoked" with the
// tt-backend truth ("running"), re-granting access to a revoked learner. This
// mirrors the existing StateStopped protection (terminalSyncService.go:223).
//
// RED today: the sync flips "revoked" -> "running".
func TestSyncUserSessions_RevokedRow_NotClobberedByActiveAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	userID := "revoked-sync-user"
	sessionID := "sess-revoked-sync"

	sessions := []dto.TerminalTrainerSession{
		{
			SessionID:   sessionID,
			Status:      0, // still active on tt-backend
			ExpiresAt:   time.Now().Add(1 * time.Hour).Unix(),
			State:       models.StateRunning,
			MachineSize: "S",
		},
	}
	srv := syncStubBackend(t, func() []dto.TerminalTrainerSession { return sessions })
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	require.NoError(t, db.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		State:             models.TerminalState(revokedWireState),
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}).Error)

	svc := services.NewTerminalTrainerService(db)
	_, err = svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var got models.Terminal
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&got).Error)
	assert.Equal(t, revokedWireState, string(got.State),
		"a revoked terminal must stay revoked through a sync pass — the billing revocation is "+
			"authoritative in ocf-core and must not be reconciled back to the tt-backend truth")
}

// TestSyncUserSessions_ExpiredSession_StillDeletedNotRevoked is a guard: only
// the billing-revocation path may produce "revoked". A normal TTL expiry — a
// local row that no longer appears in the tt-backend list — must still become
// "deleted", never "revoked". This pins that adding the revoked state does not
// bleed into the ordinary expiry/reap path.
//
// GREEN today and must stay green after the revoked contract lands.
func TestSyncUserSessions_ExpiredSession_StillDeletedNotRevoked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	userID := "expiry-guard-user"
	sessionID := "sess-expiry-guard"

	// Empty list — the container was reaped by tt-backend (normal expiry).
	srv := syncStubBackend(t, func() []dto.TerminalTrainerSession { return nil })
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	require.NoError(t, db.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		State:             models.StateRunning,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}).Error)

	svc := services.NewTerminalTrainerService(db)
	_, err = svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var got models.Terminal
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&got).Error)
	assert.Equal(t, models.StateDeleted, got.State,
		"a normal TTL expiry (reaped on tt-backend) must remain 'deleted', not 'revoked' — "+
			"revoked is reserved for the billing-revocation cleanup path only")
}

// TestSyncUserSessions_RevokedRow_NotClobberedByStoppedContainer pins the SECOND
// clobber path (found in review of !288). The specialized markSessionStopped
// branch (terminalSyncService.go:277, `if apiSession.State == StateStopped`) is
// NOT gated by the localStateIsAuthoritative guard that protects the other two
// propagation sites (:234, :286). This path is reachable for a revoked session:
// billing revocation leaves the container alive and the UserTerminalKey active,
// so the sync keeps running and tt-backend's idle reaper eventually reports the
// still-alive container as "stopped". Unguarded, the revoked row is flipped to
// StateStopped — which re-occupies budget (StateStopped ∈
// TerminalStatesOccupyingSlot) AND re-exposes the frontend Resume button,
// handing a revoked learner their session back mid-revocation.
//
// RED today: markSessionStopped resurrects "revoked" as "stopped".
func TestSyncUserSessions_RevokedRow_NotClobberedByStoppedContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	userID := "revoked-stopped-user"
	sessionID := "sess-revoked-stopped"

	// tt-backend idle-auto-stopped the still-alive container.
	idleUntil := time.Now().Add(20 * time.Minute)
	sessions := []dto.TerminalTrainerSession{
		{
			SessionID:       sessionID,
			Status:          0,
			ExpiresAt:       time.Now().Add(1 * time.Hour).Unix(),
			State:           models.StateStopped,
			PersistenceMode: "persistent",
			IdleUntil:       idleUntil.Unix(),
		},
	}
	srv := syncStubBackend(t, func() []dto.TerminalTrainerSession { return sessions })
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	require.NoError(t, db.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		State:             models.StateRevoked,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}).Error)

	svc := services.NewTerminalTrainerService(db)
	_, err = svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var got models.Terminal
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&got).Error)
	assert.Equal(t, models.StateRevoked, got.State,
		"a revoked terminal must survive an idle auto-stop on tt-backend — the stopped-container "+
			"sync branch must not resurrect a revoked session as stopped (resumable + budget-occupying)")

	// Cheap bonus: still frees the slot/budget. If the row were flipped to
	// StateStopped (with the future idle_until as ExpiresAt) it would re-enter
	// OccupiesSlotScope and count as 1.
	occupied, err := models.CountUserOccupiedSlots(db, userID, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), occupied,
		"a revoked session must not re-occupy a budget slot after an idle auto-stop sync")
}

// TestSyncUserSessions_StoppedRow_StillRefreshesIdleDeadline is the mirror guard:
// the fix for the revoked clobber above must NOT break the legitimate path. A
// genuinely-stopped local row whose tt-backend counterpart reports a fresh
// idle_until must still adopt it via markSessionStopped and stay stopped. This
// constrains the fix to guard specifically against StateRevoked at
// terminalSyncService.go:277 — NOT against the whole localStateIsAuthoritative
// set, since StateStopped is authoritative too but must keep flowing through
// markSessionStopped to refresh its resume window.
//
// GREEN today and must stay green after the guard lands.
func TestSyncUserSessions_StoppedRow_StillRefreshesIdleDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	userID := "stopped-refresh-user"
	sessionID := "sess-stopped-refresh"

	idleUntil := time.Now().Add(25 * time.Minute).Truncate(time.Second)
	sessions := []dto.TerminalTrainerSession{
		{
			SessionID:       sessionID,
			Status:          0,
			ExpiresAt:       time.Now().Add(-1 * time.Minute).Unix(), // stale create deadline
			State:           models.StateStopped,
			PersistenceMode: "persistent",
			IdleUntil:       idleUntil.Unix(),
		},
	}
	srv := syncStubBackend(t, func() []dto.TerminalTrainerSession { return sessions })
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Already stopped locally, with a stale ExpiresAt that must be refreshed.
	require.NoError(t, db.Create(&models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		State:             models.StateStopped,
		PersistenceMode:   "persistent",
		ExpiresAt:         time.Now().Add(-1 * time.Minute),
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}).Error)

	svc := services.NewTerminalTrainerService(db)
	_, err = svc.SyncUserSessions(userID)
	require.NoError(t, err)

	var got models.Terminal
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&got).Error)
	assert.Equal(t, models.StateStopped, got.State,
		"a genuinely-stopped row must stay stopped")
	assert.WithinDuration(t, idleUntil, got.ExpiresAt, 2*time.Second,
		"a stopped row must still adopt the tt-backend idle_until as its ExpiresAt — the revoked "+
			"guard must not block markSessionStopped for legit stopped rows")
}
