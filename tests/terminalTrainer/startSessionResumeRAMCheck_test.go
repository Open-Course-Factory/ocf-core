// tests/terminalTrainer/startSessionResumeRAMCheck_test.go
//
// Regression coverage for the Resume flow's host-capacity check.
//
// Symptom of the original user-reported bug: POST /api/v1/terminals/:id/start
// (Resume) was returning 503 "Server at capacity. Please try again later."
// even for a tiny session, because production's CheckRAMAvailability
// middleware read CreateComposedSessionInput.Size from the JSON body. The
// resume frontend posts no body, so chosenSize = "" and resolveRequiredRAM
// fell back to plan-max (LargestSize = 4 GiB / XL). A user resuming an XS
// session got checked against 4 GiB needed, 503'd whenever host headroom
// was below that — even though their actual session only needs 256 MiB.
//
// The fix is NOT to drop the host check entirely: paused/stopped LXC
// containers DO free their RAM (incus stop terminates processes, kernel
// reclaims pages), so incus start re-allocates fresh RAM. A capacity
// check on resume IS legitimate. The fix is to evaluate the check against
// the session's PERSISTED MachineSize (set at creation, never modified),
// not against an empty request body.
//
// Contract pinned by these tests:
//
//  1. Resume of a small (XS) session SUCCEEDS even when host can fit XS but
//     not XL — proves we no longer fall back to the plan-max estimate.
//
//  2. Resume of a large session 503s when host genuinely cannot fit it —
//     proves we did not lose the legitimate guard against OOM.
//
// Implementation: the check runs inside the StartSession controller
// (after it has loaded the Terminal row for ownership), evaluating
// EvaluateLaunchCapacity against terminal.MachineSize.
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

// newResumeTTBackend stands up a fake tt-backend that mirrors the surface
// the Resume flow exercises (sessions list, sessions/{id}/start), with a
// configurable /1.0/metrics response so each test can pin a specific host
// pressure scenario.
//
//   - ramAvailableGB: the RAM headroom the fake reports.
//   - ramPercent: the host utilisation the fake reports.
//
// Pair (1.0, 75) means a host with ~4 GB total RAM, 25% free, 75% used —
// it can fit an XS (256 MiB) but cannot fit an XL (4 GiB).
// Pair (0.04, 99) means a host effectively saturated — even XS is refused.
func newResumeTTBackend(t *testing.T, sessionID string, ramAvailableGB, ramPercent float64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/metrics":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ram_available_gb": ramAvailableGB,
				"ram_percent":      ramPercent,
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
						"name":             "resume-target",
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
// production middleware/handler — so the size-aware capacity check that
// lives inside the StartSession controller is exercised under HTTP.
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

// seedResumeTarget creates a stopped persistent terminal with the given
// MachineSize so the test can exercise resume with a specific persisted
// size. Returns the session ID to drive the HTTP request against.
func seedResumeTarget(t *testing.T, userID string, machineSize string) string {
	t.Helper()
	db := freshTestDB(t)

	// Plan with a non-trivial MaxMemoryMB so the body-less request would,
	// under the old buggy path, estimate a LargestSize (XL = 4 GiB)
	// allocation. The fix evaluates against the persisted size instead.
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

	sessionID := "resume-" + machineSize + "-session"
	stopped := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "ResumeTarget",
		State:             models.StateStopped,
		PersistenceMode:   "persistent",
		ExpiresAt:         time.Now().Add(-time.Minute),
		InstanceType:      "",
		MachineSize:       machineSize,
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(stopped).Error)
	return sessionID
}

// TestStartSession_Resume_XSAllowedWhenHostFitsXS_Even_If_It_DoesNotFit_XL
// is the regression test for the user-reported 503 bug.
//
// Setup: stopped persistent terminal with MachineSize="xs" (256 MiB). The
// host reports 1 GB available / 75% used — enough room for an XS, but
// nowhere near enough for the plan's max-size XL (4 GiB).
//
// Pre-fix behaviour (CheckRAMAvailability middleware on the route, body-less
// request): chosenSize="", resolveRequiredRAM falls back to LargestSize
// (4 GiB), check fails, 503.
//
// Post-fix behaviour (size-aware check using persisted MachineSize="xs"):
// 256 MiB required vs 1 GB available, check passes, request reaches handler.
func TestStartSession_Resume_XSAllowedWhenHostFitsXS_Even_If_It_DoesNotFit_XL(t *testing.T) {
	userID := "resume-xs-user"
	sessionID := seedResumeTarget(t, userID, "xs")

	// 1 GB available / 75% used — fits XS (256 MiB) comfortably, cannot
	// fit XL (4 GiB).
	ttSrv := newResumeTTBackend(t, sessionID, 1.0, 75.0)
	configureTTServer(t, ttSrv.URL)

	realSvc := services.NewTerminalTrainerService(sharedTestDB)
	router := setupResumeRouterWithProdMiddleware(t, userID, realSvc)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/terminals/"+sessionID+"/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusServiceUnavailable, w.Code,
		"Resume of an XS session must not 503 just because the host can't fit "+
			"the plan's max-size XL. The check must evaluate against the persisted "+
			"MachineSize. Got %d. Body: %s", w.Code, w.Body.String())
	assert.Equal(t, http.StatusOK, w.Code,
		"Resume of an XS session on a host with 1 GB free should succeed. "+
			"Got %d. Body: %s", w.Code, w.Body.String())
}

// TestStartSession_Resume_503sWhenHostCannotFitActualSize proves the
// legitimate guard against OOM remains. A session resuming into a host
// that genuinely cannot fit its persisted size must still get a 503 —
// otherwise the host risks running out of memory when incus start
// allocates fresh pages.
//
// Setup: stopped persistent terminal with MachineSize="xl" (4 GiB). Host
// reports 0.04 GB available / 99% used — RAM saturated, even an XS is
// refused per EvaluateLaunchCapacity's ram_full short-circuit.
//
// Expected: 503 with the same error shape the middleware produced, so the
// frontend error-handling code stays bit-compatible.
func TestStartSession_Resume_503sWhenHostCannotFitActualSize(t *testing.T) {
	userID := "resume-xl-user"
	sessionID := seedResumeTarget(t, userID, "xl")

	// 0.04 GB available / 99% used — host is RAM-saturated. Any resume
	// must be refused per the ram_full short-circuit in
	// EvaluateLaunchCapacity.
	ttSrv := newResumeTTBackend(t, sessionID, 0.04, 99.0)
	configureTTServer(t, ttSrv.URL)

	realSvc := services.NewTerminalTrainerService(sharedTestDB)
	router := setupResumeRouterWithProdMiddleware(t, userID, realSvc)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/terminals/"+sessionID+"/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code,
		"Resume of an XL session on a saturated host (99%% RAM used, 0.04 GB "+
			"free) must 503 — the host cannot satisfy the allocation. Got %d. "+
			"Body: %s", w.Code, w.Body.String())

	// Match the bit-compatible error shape from ramCheckMiddleware.go
	// so frontend error handling stays uniform across the resume and
	// create paths.
	body := w.Body.String()
	assert.Contains(t, body, "Server at capacity",
		"503 body must contain the canonical 'Server at capacity' string so "+
			"frontend error handling stays uniform. Got %q", body)
}
