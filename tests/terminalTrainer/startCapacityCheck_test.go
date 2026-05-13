// tests/terminalTrainer/startCapacityCheck_test.go
//
// History: this file originally asserted that `POST /terminals/:id/start`
// carries `CheckLimit("concurrent_terminals")` to defeat a stop/start
// "slot resurrection" bypass. After the SSOT tightening of
// `OccupiesSlotScope` (active+stopped both count), the bypass becomes
// impossible at create time: the user can no longer reach a 2-session
// state with cap=1, because the second create attempt is denied while
// the first session is still stopped.
//
// Resume itself is slot-neutral and now has NO `CheckLimit` on the
// route (see startResumeAtLimit_test.go for the positive assertion).
// The defense moved to the create gate (`POST /start-composed-session`)
// and to the SSOT slot helper, which is now the single home for the
// occupied-slot count.
//
// This file keeps a single residual assertion: at the moment a user is
// at the slot-occupancy limit, a fresh-create attempt MUST be denied
// with 403 by the CheckLimit middleware on /start-composed-session.
// The "resurrect via /start" angle is covered by the SSOT itself —
// stopped sessions are counted, so the user is at cap, and `start`
// transitions an already-counted session without changing the count.
package terminalTrainer_tests

import (
	"bytes"
	"encoding/json"
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
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// TestCreateRoute_DeniesWhenAtConcurrentCap pins the create-gate side
// of the slot-occupancy invariant: when a user has 1 stopped session
// and cap=1, the SSOT counts them as 1/1, and POST
// /terminals/start-composed-session must respond 403.
//
// This replaces the legacy assertion about /:id/start being CheckLimit-gated.
// /:id/start is slot-neutral and intentionally NOT gated (see
// startResumeAtLimit_test.go).
func TestCreateRoute_DeniesWhenAtConcurrentCap(t *testing.T) {
	db := freshTestDB(t)
	userID := "create-cap-user"

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

	// One stopped session, future expiry → counted by OccupiesSlotScope → 1/1.
	stopped, err := createTestTerminal(db, userID, "stopped", time.Now().Add(time.Hour))
	require.NoError(t, err)
	stopped.State = "stopped"
	require.NoError(t, db.Save(stopped).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})

	effectivePlanService := paymentServices.NewEffectivePlanService(sharedTestDB)
	ctrl := terminalController.NewTerminalController(sharedTestDB)

	router.POST("/api/v1/terminals/start-composed-session",
		paymentMiddleware.InjectOrgContext(),
		paymentMiddleware.InjectEffectivePlan(effectivePlanService, sharedTestDB),
		paymentMiddleware.RequirePlan(),
		paymentMiddleware.CheckLimit(effectivePlanService, sharedTestDB, "concurrent_terminals"),
		ctrl.StartComposedSession,
	)

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

	assert.Contains(t, []int{http.StatusForbidden, http.StatusTooManyRequests}, w.Code,
		"POST /terminals/start-composed-session must reject fresh-create at the "+
			"slot-occupancy cap (user has 1 stopped session, cap=1) — got %d. "+
			"This is the only path that adds a slot; CheckLimit must stay here. "+
			"Body: %s", w.Code, w.Body.String())
}
