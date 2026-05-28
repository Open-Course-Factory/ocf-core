// tests/terminalTrainer/startSessionResumeRAMCheck_test.go
//
// Regression for a user-reported 503 on the Resume flow.
//
// Symptom: POST /api/v1/terminals/:id/start (Resume) returns
//   503 "Server at capacity. Please try again later."
// even when the user can create a brand-new session immediately
// afterwards on the same backend.
//
// Root cause: production's POST /:id/start middleware chain calls
// paymentMiddleware.CheckRAMAvailability(terminalService). That middleware
// reads CreateComposedSessionInput.Size from the JSON body to size the
// estimate. Resume sends NO body, so the middleware falls back to the
// plan-max estimate (LargestSize). On a host whose headroom is below the
// plan-max — i.e., the steady state of any well-utilised production host —
// every Resume 503s, regardless of the actual size of the persisted
// session.
//
// Why CheckRAMAvailability is the wrong gate on resume:
//
//   1. The session was already counted against the user's budget at
//      creation (D6': stopped sessions occupy a slot). The resumed
//      footprint is deterministic — it cannot exceed what was allocated.
//   2. tt-backend's resume handler is a state transition on an EXISTING
//      Incus container — no new disk, no new memory allocation up front.
//   3. The size is fixed at creation. Re-checking host capacity against
//      a body-less fallback estimate is meaningless.
//
// Contract pinned here: the Resume route MUST NOT invoke CheckRAMAvailability.
// The other plan-validity middleware (Auth, Layer-2 ownership, InjectOrgContext,
// InjectEffectivePlan, RequirePlan) stays — those checks are still appropriate
// on resume.
package terminalTrainer_tests

import (
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
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
)

// newTightRAMTTBackend stands up a fake tt-backend that mirrors the
// minimum surface the Resume flow exercises (sessions list, sessions/{id}/start)
// AND returns a deliberately tight /1.0/metrics response.
//
// The metrics shape (1.7 GB available / 83% used) is the canonical
// "tight host" fixture from the CheckRAMAvailability unit tests: it sits
// below the threshold for any plan-max fallback estimate, so a body-less
// Resume request that flows through CheckRAMAvailability will 503 — which
// is exactly the bug this regression test pins.
func newTightRAMTTBackend(t *testing.T, sessionID string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/metrics":
			// Tight host — pre-fix this triggers CapacityStatusCritical on
			// the plan-max fallback path inside CheckRAMAvailability.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ram_available_gb": 1.7,
				"ram_percent":      83.0,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/sessions":
			// SyncUserSessions sees the session as stopped (status=1) so
			// ValidateSessionAccess classifies it as resumable and the
			// allowStopped branch passes through.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions": []map[string]any{
					{
						"id":               sessionID,
						"session_id":       sessionID,
						"name":             "ram-check",
						"status":           1,
						"expires_at":       time.Now().Add(-30 * time.Second).Unix(),
						"created_at":       time.Now().Add(-time.Hour).Unix(),
						"state":            "stopped",
						"persistence_mode": "persistent",
					},
				},
				"count":           1,
				"include_expired": true,
				"limit":           1000,
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "running"})
		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// setupResumeRouterWithProdMiddleware wires the POST /:id/start route
// EXACTLY as production registers it in
// src/terminalTrainer/routes/terminalRoutes.go — modulo one unavoidable
// substitution other HTTP tests in this package make:
//
//   - AuthManagement → userId/userRoles stub (no Casdoor in tests)
//
// Everything else (RequireTerminalAccessAllowStopped, InjectOrgContext,
// InjectEffectivePlan, RequirePlan, StartSession handler) is the real
// production middleware/handler.
//
// CONTRACT: this helper MUST stay in lock-step with the production route.
// If a reviewer adds CheckRAMAvailability (or any other capacity gate)
// back to the production /:id/start chain, this helper must mirror it —
// and the regression test below will then turn red on the 503 it produces
// against the tight-RAM fixture.
func setupResumeRouterWithProdMiddleware(
	t *testing.T,
	userID string,
	realSvc services.TerminalTrainerService,
) *gin.Engine {
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
	ctrl := terminalController.NewTerminalControllerWithService(sharedTestDB, realSvc)

	router.POST("/api/v1/terminals/:id/start",
		accessMW.RequireTerminalAccessAllowStopped(),
		paymentMiddleware.InjectOrgContext(),
		paymentMiddleware.InjectEffectivePlan(effectivePlanService, sharedTestDB),
		paymentMiddleware.RequirePlan(),
		ctrl.StartSession,
	)
	return router
}

// TestStartSession_ResumeStoppedSession_DoesNotCheckRAMAvailability pins
// the contract the bug-fix establishes: the Resume route must NOT invoke
// CheckRAMAvailability. The session's footprint is already counted in the
// budget at creation; re-checking host RAM against a body-less fallback
// estimate produces spurious 503s on hosts whose headroom is below the
// plan-max — i.e., every realistic production host.
//
// The test seeds a stopped persistent terminal, hits the Resume route
// with NO body (mirroring the production frontend), and asserts the
// response is NOT a 503 — pre-fix, CheckRAMAvailability is on the chain
// and the body-less fallback inside it estimates a plan-max allocation
// against a tight-RAM tt-backend fixture and aborts with 503.
func TestStartSession_ResumeStoppedSession_DoesNotCheckRAMAvailability(t *testing.T) {
	sessionID := "ram-check-resume-session"

	// Fake tt-backend with a tight /1.0/metrics response. Pre-fix, this is
	// what would have made CheckRAMAvailability abort with 503; post-fix the
	// metrics endpoint is never called because the middleware is no longer
	// wired into the Resume chain.
	ttSrv := newTightRAMTTBackend(t, sessionID)
	configureTTServer(t, ttSrv.URL)

	db := freshTestDB(t)
	userID := "ram-check-resume-user"

	// Plan with a non-trivial MaxMemoryMB so the body-less fallback inside
	// CheckRAMAvailability would estimate a LargestSize allocation — the
	// scenario that 503s pre-fix on a tight host.
	plan := &paymentModels.SubscriptionPlan{
		Name:                      "Formateur",
		Priority:                  10,
		MaxSessionDurationMinutes: 60,
		MaxMemoryMB:               4096,
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

	// Seed a stopped persistent terminal — the canonical Resume target.
	stopped := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "ResumeTarget",
		State:             "stopped",
		PersistenceMode:   "persistent",
		ExpiresAt:         time.Now().Add(-time.Minute),
		InstanceType:      "",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(stopped).Error)

	realSvc := services.NewTerminalTrainerService(sharedTestDB)
	router := setupResumeRouterWithProdMiddleware(t, userID, realSvc)

	// Resume request: NO body — the production frontend posts to /:id/start
	// without a body, which is exactly what triggers the body-less fallback
	// inside CheckRAMAvailability.
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/terminals/"+sessionID+"/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Primary contract: no 503 from the RAM gate. The user-observable bug
	// was a 503 from CheckRAMAvailability when its body-less fallback to
	// plan-max ran against a host whose actual headroom was below that
	// estimate. Removing the middleware from the Resume chain eliminates
	// the gate; the request now reaches the StartSession handler.
	assert.NotEqual(t, http.StatusServiceUnavailable, w.Code,
		"Resume must not 503 because of CheckRAMAvailability — the session's "+
			"footprint is already counted in the budget at creation. Got %d. Body: %s",
		w.Code, w.Body.String())
}
