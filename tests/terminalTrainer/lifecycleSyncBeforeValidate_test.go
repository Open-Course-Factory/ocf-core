// tests/terminalTrainer/lifecycleSyncBeforeValidate_test.go
//
// Bug under test (Part A): ocf-core's terminals.state lags tt-backend's
// authoritative state because only GET /terminals/user-sessions auto-syncs
// today. Lifecycle endpoints (POST /:id/start, DELETE /:id) read from
// the cached row, so a tt-backend auto-stop that fired in the last few
// seconds can produce a stale 'running'/'expired' verdict and 410 the
// user out of Resuming their own session.
//
// Fix: the RequireTerminalAccessAllowStopped middleware now calls
// SyncUserSessions BEFORE ValidateSessionAccess. Sync failures are tolerated
// (logged, fall through to the cached row) so a tt-backend hiccup does not
// block lifecycle operations.
//
// This file pins the call ordering behaviourally: the test stands up a fake
// tt-backend that records every request path, drives a Resume of a stale
// local row, and asserts:
//
//   - the middleware did NOT 410 the request (i.e., the cache got refreshed)
//   - the start handler reached tt-backend's POST /sessions/{id}/start
//   - the sync GET happened BEFORE the start POST (the "before validate" claim)
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	terminalMiddleware "soli/formations/src/terminalTrainer/middleware"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
)

// zombieTTBackendStub captures the ordered list of paths a test exercises
// on the fake tt-backend. It returns a /1.0/sessions listing whose
// `state` / `status` reflect a session that tt-backend already auto-stopped,
// and responds 200 to POST /sessions/{id}/start so the handler can complete.
type zombieTTBackendStub struct {
	mu             sync.Mutex
	sessionID      string
	expiresAt      int64
	pathsHit       []string
	syncCalls      int
	startCalls     int
	stateAfterSync string // what /sessions reports for `state` after the auto-stop
}

func newZombieTTBackend(t *testing.T, sessionID string, stateAfterSync string) (*httptest.Server, *zombieTTBackendStub) {
	t.Helper()
	stub := &zombieTTBackendStub{
		sessionID:      sessionID,
		expiresAt:      time.Now().Add(-30 * time.Second).Unix(),
		stateAfterSync: stateAfterSync,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		stub.pathsHit = append(stub.pathsHit, r.Method+" "+r.URL.Path)
		stub.mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/sessions":
			stub.mu.Lock()
			stub.syncCalls++
			// SyncUserSessions maps SessionStatus: 0 → "running",
			// anything else → "deleted". Send status=1 when the desired
			// outcome is a non-running state so the sync path triggers.
			apiStatus := 0
			respState := stub.stateAfterSync
			if respState == "" {
				respState = "running"
			}
			if respState != "running" {
				apiStatus = 1
			}
			payload := map[string]any{
				"sessions": []map[string]any{
					{
						"id":               stub.sessionID,
						"session_id":       stub.sessionID,
						"name":             "zombie",
						"status":           apiStatus,
						"expires_at":       stub.expiresAt,
						"created_at":       time.Now().Add(-time.Hour).Unix(),
						"ip":               "10.0.0.42",
						"state":            respState,
						"persistence_mode": "persistent",
					},
				},
				"count":           1,
				"api_key_id":      0,
				"include_expired": true,
				"limit":           1000,
			}
			stub.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start"):
			stub.mu.Lock()
			stub.startCalls++
			stub.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "running"})
		default:
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, stub
}

// setupResumeRouterWithSync wires the /:id/start route exactly the way
// production does for the path under test (modulo AuthManagement and
// CheckRAMAvailability — see setupResumeRouter in startResumeAtLimit_test.go
// for the standard test substitutions). The crucial bit:
// RequireTerminalAccessAllowStopped is the REAL middleware; the test asserts
// that Part A wires sync-before-validate through it.
func setupResumeRouterWithSync(t *testing.T, userID string, svc services.TerminalTrainerService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})

	effectivePlanService := paymentServices.NewEffectivePlanService(sharedTestDB)
	accessMW := terminalMiddleware.NewTerminalAccessMiddleware(sharedTestDB)
	ctrl := terminalController.NewTerminalControllerWithService(sharedTestDB, svc)

	router.POST("/api/v1/terminals/:id/start",
		accessMW.RequireTerminalAccessAllowStopped(),
		paymentMiddleware.InjectOrgContext(),
		paymentMiddleware.InjectEffectivePlan(effectivePlanService, sharedTestDB),
		paymentMiddleware.RequirePlan(),
		ctrl.StartSession,
	)

	return router
}

// TestResumeMiddleware_SyncsBeforeValidate exercises Part A end-to-end.
//
// Setup:
//   - Local terminals row: state='running', ExpiresAt past, PersistenceMode='persistent'.
//     This is the stale cache from before tt-backend's auto-stop completed.
//   - Fake tt-backend's /1.0/sessions now reports state='stopped' / status=1
//     for the same session — the auto-stop has finished on the BE side.
//
// With Part A:
//   - Middleware calls SyncUserSessions first; the local row is updated to
//     state='stopped'.
//   - ValidateSessionAccess sees state='stopped' → reason='stopped'.
//   - allowStopped=true → middleware lets the request through.
//   - StartSession handler runs and POSTs to tt-backend's /start.
//
// Assertions are behavioural: the request was not 410'd, and the tt-backend
// stub observed the sync GET BEFORE the start POST.
func TestResumeMiddleware_SyncsBeforeValidate(t *testing.T) {
	sessionID := "zombie-resume-session"
	ttSrv, stub := newZombieTTBackend(t, sessionID, "stopped")
	configureTTServer(t, ttSrv.URL)

	db := freshTestDB(t)
	userID := "zombie-resume-user"

	// Plan with persistence enabled — the route uses InjectEffectivePlan.
	plan := &paymentModels.SubscriptionPlan{
		Name:                      "Formateur",
		Priority:                  10,
		MaxSessionDurationMinutes: 60,
		DataPersistenceEnabled:    true,
		IsActive:                  true,
		BillingInterval:           "month",
		Currency:                  "eur",
	}
	require.NoError(t, db.Create(plan).Error)
	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		SubscriptionType:   "personal",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}).Error)

	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Seed the STALE local row: state='running', past ExpiresAt, persistent.
	stale := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "Zombie",
		State:             "running",
		PersistenceMode:   "persistent",
		ExpiresAt:         time.Now().Add(-30 * time.Second),
		InstanceType:      "",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(stale).Error)

	svc := services.NewTerminalTrainerService(sharedTestDB)
	router := setupResumeRouterWithSync(t, userID, svc)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/terminals/"+sessionID+"/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Primary behavioural assertion: middleware did NOT 410 the request.
	assert.NotEqual(t, http.StatusGone, w.Code,
		"middleware must not reject the Resume of a zombie persistent session as 'expired'; "+
			"the pre-validate sync should pick up tt-backend's auto-stop and let allowStopped pass it through. "+
			"Got %d. Body: %s", w.Code, w.Body.String())

	stub.mu.Lock()
	syncCalls := stub.syncCalls
	startCalls := stub.startCalls
	pathsHit := append([]string(nil), stub.pathsHit...)
	stub.mu.Unlock()

	assert.GreaterOrEqual(t, syncCalls, 1,
		"middleware must call SyncUserSessions (which hits GET /1.0/sessions) before validating; "+
			"tt-backend stub recorded %d sync calls and these paths: %v", syncCalls, pathsHit)
	assert.GreaterOrEqual(t, startCalls, 1,
		"the start handler must reach tt-backend's POST /sessions/{id}/start — "+
			"if the middleware short-circuited, this would be 0. Paths: %v", pathsHit)

	// Order check: sync GET must precede start POST. The start POST only
	// fires after validation passes, so observing them in that order proves
	// the sync ran BEFORE the validate.
	require.GreaterOrEqual(t, len(pathsHit), 2,
		"expected at least one sync GET and one start POST, got paths: %v", pathsHit)
	firstSyncIdx, firstStartIdx := -1, -1
	for i, p := range pathsHit {
		if firstSyncIdx == -1 && strings.HasPrefix(p, "GET /1.0/sessions") {
			firstSyncIdx = i
		}
		if firstStartIdx == -1 && strings.HasPrefix(p, "POST ") && strings.HasSuffix(p, "/start") {
			firstStartIdx = i
		}
	}
	require.NotEqual(t, -1, firstSyncIdx, "no sync GET observed; paths: %v", pathsHit)
	require.NotEqual(t, -1, firstStartIdx, "no start POST observed; paths: %v", pathsHit)
	assert.Less(t, firstSyncIdx, firstStartIdx,
		"sync GET must come BEFORE the start POST (Part A: sync before validate). Paths: %v", pathsHit)
}
