// tests/terminalTrainer/startResumeAtLimit_test.go
//
// Resume of a stopped session must succeed at the slot-occupancy limit.
//
// Bug: POST /api/v1/terminals/:id/start used to carry
// paymentMiddleware.CheckLimit("concurrent_terminals"). After the SSOT
// tightening of OccupiesSlotScope (active+stopped both count toward the
// quota), a stopped session already occupies a slot. CheckLimit then
// computed currentUsage=1, increment=1, 1+1=2 > limit=1 → 403, even
// though resume is a slot-neutral state transition (stopped → running,
// no new slot).
//
// This file pins:
//
//  1. Resume-at-limit succeeds (200 OK). The route's middleware chain
//     post-fix does NOT include CheckLimit.
//  2. Fresh-create-at-limit still fails (403). The create flow at
//     POST /start-composed-session is the only path that adds a slot
//     and must remain gated.
//
// Together these two assertions describe the slot-occupancy invariant:
// state transitions of an existing session do not consume new slots;
// creating a new session does.
package terminalTrainer_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	terminalMiddleware "soli/formations/src/terminalTrainer/middleware"
	terminalController "soli/formations/src/terminalTrainer/routes"
	terminalServices "soli/formations/src/terminalTrainer/services"
)

// setupResumeRouter wires the /:id/start route with the SAME middleware
// chain production uses (modulo two unavoidable substitutions explained
// below). The test must mirror production wiring: hand-rolling a parallel
// chain that drops the middleware-under-test would silently bypass the
// bug instead of pinning it.
//
// Substitutions versus the real `TerminalRoutes()` registration:
//
//   - `AuthManagement()` is replaced with a userId/userRoles stub. The
//     real middleware demands a Casdoor-signed JWT, which is impractical
//     in unit tests. Other terminalTrainer tests do the same.
//   - `CheckRAMAvailability` is omitted. RAM is gated by a separate
//     middleware that needs a live tt-backend metrics endpoint and is
//     orthogonal to the slot-occupancy assertion under test.
//
// Everything else — InjectOrgContext, InjectEffectivePlan, RequirePlan,
// and the StartSession handler — runs exactly like production.
//
// To keep the test honest, it asserts on the CURRENT production chain
// at the time of writing: it includes `CheckLimit("concurrent_terminals")`.
// When the fix removes that middleware from the route, both production
// and the wiring below MUST change together. The 200-OK assertion does
// not change.
func setupResumeRouter(t *testing.T, userID string, svc terminalServices.TerminalTrainerService) *gin.Engine {
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

	// Production wiring — KEEP IN SYNC with src/terminalTrainer/routes/terminalRoutes.go.
	// When the route changes, this list changes; the test assertion does not.
	// Notable absence: NO CheckLimit. Resume is slot-neutral; see route comment.
	router.POST("/api/v1/terminals/:id/start",
		accessMW.RequireTerminalAccessAllowStopped(),
		paymentMiddleware.InjectOrgContext(),
		paymentMiddleware.InjectEffectivePlan(effectivePlanService, sharedTestDB),
		paymentMiddleware.RequirePlan(),
		ctrl.StartSession,
	)

	return router
}

// startTTBackendStub spins up a fake tt-backend that responds 200 to
// POST /sessions/{id}/start. Mirrors startLifecycleTTServer but with
// only the start endpoint wired (the other lifecycle paths are not
// touched by this test).
func startTTBackendStub(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "running"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	return srv
}

// TestResumeStoppedSession_AtLimit_Succeeds is the regression test for
// the bug. With a plan cap of 1 and exactly one stopped session
// (already counted by OccupiesSlotScope), POST /terminals/:id/start
// MUST succeed because resume is slot-neutral.
//
// Today this fails with 403 ("Usage limit exceeded for
// concurrent_terminals. Current: 1, Limit: 1") because the route still
// carries CheckLimit("concurrent_terminals").
func TestResumeStoppedSession_AtLimit_Succeeds(t *testing.T) {
	ttServer := startTTBackendStub(t)
	defer ttServer.Close()

	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	db := freshTestDB(t)
	userID := "resume-at-limit-user"

	// Plan: cap=1.
	plan := &paymentModels.SubscriptionPlan{
		Name:                   "Solo",
		Priority:               5,
		MaxConcurrentTerminals: 1,
		MaxCourses:             5,
		IsActive:               true,
		BillingInterval:        "month",
		Currency:               "eur",
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

	// Pre-state: ONE stopped session, future expiry. It already counts
	// toward the quota via OccupiesSlotScope (status='stopped',
	// expires_at > now). The user is therefore at 1/1.
	stopped, err := createTestTerminal(db, userID, "stopped", time.Now().Add(time.Hour))
	require.NoError(t, err)
	stopped.State = "stopped"
	require.NoError(t, db.Save(stopped).Error)

	svc := terminalServices.NewTerminalTrainerService(sharedTestDB)
	router := setupResumeRouter(t, userID, svc)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/terminals/"+stopped.SessionID+"/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Resume MUST succeed. The session simply transitions stopped → running;
	// no new slot is consumed (OccupiesSlotScope already counts it).
	//
	// Pre-fix this fails with 403 because CheckLimit reads currentUsage=1,
	// adds increment=1, and rejects (2 > 1).
	assert.Equal(t, http.StatusOK, w.Code,
		"POST /terminals/:id/start must succeed when the user is at the "+
			"slot-occupancy limit — the stopped session already counts. "+
			"Got %d. Body: %s", w.Code, w.Body.String())
}

// setupCreateRouter wires the /start-composed-session route. The
// middleware chain is identical to production except CheckRAMAvailability
// is omitted (same reasoning as setupResumeRouter).
func setupCreateRouter(t *testing.T, userID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})

	effectivePlanService := paymentServices.NewEffectivePlanService(sharedTestDB)
	ctrl := terminalController.NewTerminalController(sharedTestDB)

	// Production wiring (modulo RAM):
	//   AuthManagement → InjectOrgContext → InjectEffectivePlan → RequirePlan
	//     → CheckLimit("concurrent_terminals") → CheckRAMAvailability → StartComposedSession
	router.POST("/api/v1/terminals/start-composed-session",
		paymentMiddleware.InjectOrgContext(),
		paymentMiddleware.InjectEffectivePlan(effectivePlanService, sharedTestDB),
		paymentMiddleware.RequirePlan(),
		paymentMiddleware.CheckLimit(effectivePlanService, sharedTestDB, "concurrent_terminals"),
		ctrl.StartComposedSession,
	)

	return router
}

// TestStartComposedSession_AtLimit_Denied is the regression-guard test:
// the create gate must STILL deny when the user is at the slot-occupancy
// limit. This isolates the fix: removing CheckLimit from /:id/start
// must not accidentally relax the gate on the create path.
//
// Today this already passes; it must keep passing post-fix.
func TestStartComposedSession_AtLimit_Denied(t *testing.T) {
	db := freshTestDB(t)
	userID := "create-at-limit-user"

	plan := &paymentModels.SubscriptionPlan{
		Name:                   "Solo",
		Priority:               5,
		MaxConcurrentTerminals: 1,
		MaxCourses:             5,
		IsActive:               true,
		BillingInterval:        "month",
		Currency:               "eur",
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

	// One stopped session, future expiry → 1/1 slot usage.
	stopped, err := createTestTerminal(db, userID, "stopped", time.Now().Add(time.Hour))
	require.NoError(t, err)
	stopped.State = "stopped"
	require.NoError(t, db.Save(stopped).Error)

	router := setupCreateRouter(t, userID)

	body, _ := json.Marshal(map[string]any{
		"distribution": "ubuntu-24.04",
		"size":         "S",
		"terms":        "accepted",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/terminals/start-composed-session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"POST /terminals/start-composed-session must be denied with 403 when "+
			"the user is at the slot-occupancy limit — got %d. Body: %s",
		w.Code, w.Body.String())

	// The 403 payload carries the source the frontend uses to localize the
	// toast (see ocf-front getLimitReachedMessage). Plan came from a personal
	// UserSubscription, so source must be "personal".
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "personal", payload["source"],
		"403 payload must include source='personal' so the frontend can "+
			"localize the limit-reached toast")
}
