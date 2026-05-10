// tests/terminalTrainer/startCapacityCheck_test.go
//
// Failing-first test for the second root cause of the MaxConcurrentTerminals
// bypass: the `POST /api/v1/terminals/:id/start` route has NO capacity-check
// middleware.
//
// Route wiring today (src/terminalTrainer/routes/terminalRoutes.go:43):
//
//     routes.POST("/:id/start",
//         middleware.AuthManagement(),
//         terminalAccessMiddleware.RequireTerminalAccessAllowStopped(),
//         terminalController.StartSession)
//
// Compare with /start-composed-session (line 79) which has:
//
//     paymentMiddleware.InjectOrgContext(),
//     paymentMiddleware.InjectEffectivePlan(...),
//     paymentMiddleware.RequirePlan(),
//     paymentMiddleware.CheckLimit(..., "concurrent_terminals"),
//     paymentMiddleware.CheckRAMAvailability(...),
//
// The fix is to add (at minimum) `CheckLimit("concurrent_terminals")` to the
// /start route — OR to perform the equivalent check inside StartSession at
// the service layer. Either implementation will make this test pass.
//
// This test uses the same auth-stubbing pattern as capacityEndpoint_test.go.
// It wires the production-equivalent middleware chain for the /start route
// and asserts that POSTing to /:id/start while at the concurrent_terminals
// cap is denied with 403.
package terminalTrainer_tests

import (
	"net/http"
	"net/http/httptest"
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
)

// setupStartRouter wires a router that mirrors the production /start route's
// EXACT middleware chain plus a CheckLimit step that the route SHOULD have.
//
// The test does NOT install CheckLimit deliberately — it wires the route the
// way production wires it today. The assertion then proves that the production
// wiring lets a stop/start bypass through. The fix is to add a capacity check
// at this seam (either as middleware or inside the handler/service).
func setupStartRouter(t *testing.T, userID string, plan *paymentModels.SubscriptionPlan) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Stub auth: simulate AuthManagement() having validated a JWT.
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})

	// Subscribe the user so EffectivePlanService can resolve the plan
	// (the eventual fix may inject the plan via middleware exactly like
	// /start-composed-session does).
	router.Use(paymentMiddleware.InjectEffectivePlan(
		paymentServices.NewEffectivePlanService(sharedTestDB), sharedTestDB))
	router.Use(paymentMiddleware.RequirePlan())
	_ = plan // kept for future hookups when the fix uses inject-from-context

	accessMW := terminalMiddleware.NewTerminalAccessMiddleware(sharedTestDB)
	ctrl := terminalController.NewTerminalController(sharedTestDB)

	// THIS IS THE EXACT PRODUCTION WIRING — no CheckLimit middleware. When
	// the fix lands, this line must be updated to mirror the new production
	// wiring (e.g. add paymentMiddleware.CheckLimit(...)). At that point the
	// test will pass.
	router.POST("/terminals/:id/start",
		accessMW.RequireTerminalAccessAllowStopped(),
		ctrl.StartSession,
	)

	return router
}

// TestStartRoute_DeniesWhenAtConcurrentCap is the handler-level proof that
// the /start route allows a stop/start bypass. The user is on cap=1 with
// one running terminal and one stopped terminal. Calling /start on the
// stopped one would resurrect a second active session — the route MUST
// reject it.
func TestStartRoute_DeniesWhenAtConcurrentCap(t *testing.T) {
	db := freshTestDB(t)
	userID := "start-route-cap-user"

	// Plan: cap=1 concurrent terminal.
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

	// Pre-state: one running terminal AND one stopped terminal owned by
	// the same user. The user is already at cap. Calling /start on the
	// stopped one would bring the active count to 2 — must be denied.
	_, err := createTestTerminal(db, userID, "active", time.Now().Add(time.Hour))
	require.NoError(t, err)

	stopped, err := createTestTerminal(db, userID, "stopped", time.Now().Add(time.Hour))
	require.NoError(t, err)
	stopped.State = "stopped"
	require.NoError(t, db.Save(stopped).Error)

	router := setupStartRouter(t, userID, plan)

	// The route's :id param is matched against terminal.session_id by the
	// access middleware (ValidateSessionAccess queries WHERE session_id = ?).
	req := httptest.NewRequest(http.MethodPost,
		"/terminals/"+stopped.SessionID+"/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The fix must abort the request BEFORE the StartSession handler runs.
	// Acceptable status codes:
	//   403 — CheckLimit middleware (matches /start-composed-session)
	//   429 — rate-limit-style rejection
	// Any other code (200, 500, etc.) means the handler was invoked — i.e.
	// the request passed all gates including (the missing) capacity check.
	assert.Contains(t, []int{http.StatusForbidden, http.StatusTooManyRequests}, w.Code,
		"POST /terminals/:id/start must be denied with 403/429 when the user "+
			"is at the concurrent_terminals cap — got %d. The current behavior "+
			"reaches the handler (likely 500 from a downstream call), proving "+
			"the bypass: a stop/launch cycle resurrects a slot. Body: %s",
		w.Code, w.Body.String())
}
