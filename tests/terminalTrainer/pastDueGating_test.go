// tests/terminalTrainer/pastDueGating_test.go
//
// RED-phase gating test for issue #371 / MR !274 — past_due dunning grace.
//
// Contract (approved): once a past_due subscription is older than the grace
// period (default 7 days, env PAYMENT_PAST_DUE_GRACE_DAYS), NEW session-creation
// paths (composed-start, resume, bulk-create) are rejected with HTTP 402 and a
// stable error code `subscription_past_due`; within grace / active are allowed.
//
// This pins the primary path (composed-start) end-to-end through the production
// middleware chain. It is RED today TWICE over: (1) resolution excludes past_due
// so RequirePlan currently aborts 403 (not 402); (2) even once resolution is
// broadened, no gate emits `subscription_past_due` until the shared gate is
// wired. The assertion is on the status code + error code, not prose, so it is
// robust to whether the dev places the gate as a middleware or in the handler.
//
// The sibling resume + bulk-create paths share the same gate helper; they are
// noted in the report (heavier per-route preconditions) and can be added once
// the gate lands.
package terminalTrainer_tests

import (
	"bytes"
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
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
)

// TestComposedStart_PastDueBeyondGrace_Rejected402 drives the composed-start
// route with a past_due subscription whose grace window has elapsed and asserts
// the payment-required gate.
func TestComposedStart_PastDueBeyondGrace_Rejected402(t *testing.T) {
	db := freshTestDB(t)
	userID := "pastdue-gate-user"

	plan := &paymentModels.SubscriptionPlan{
		Name:                      "Formateur",
		Priority:                  10,
		MaxSessionDurationMinutes: 60,
		MaxMemoryMB:               8192,
		MaxCPU:                    8,
		IsActive:                  true,
		BillingInterval:           "month",
		Currency:                  "eur",
	}
	require.NoError(t, db.Create(plan).Error)

	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "past_due",
		SubscriptionType:   "personal",
		CurrentPeriodStart: time.Now().Add(-40 * 24 * time.Hour),
		CurrentPeriodEnd:   time.Now().Add(-10 * 24 * time.Hour),
	}).Error)
	// Grace window elapsed: past_due since 10 days ago (default grace 7d). No-op
	// today while the column is absent; effective once PastDueSince exists.
	_ = db.Exec("UPDATE user_subscriptions SET past_due_since = ? WHERE user_id = ?",
		time.Now().Add(-10*24*time.Hour), userID).Error

	// Wire the production chain (AuthManagement replaced by a userId stub).
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	eps := paymentServices.NewEffectivePlanService(db)
	termSvc := services.NewTerminalTrainerService(db)
	ctrl := terminalController.NewTerminalControllerWithService(db, termSvc)
	router.POST("/api/v1/terminals/start-composed-session",
		paymentMiddleware.InjectOrgContext(),
		paymentMiddleware.InjectEffectivePlan(eps, db),
		paymentMiddleware.RequirePlan(),
		paymentMiddleware.CheckRAMAvailability(termSvc),
		ctrl.StartComposedSession,
	)

	body := []byte(`{"distribution":"ubuntu-24.04","size":"L","terms":"accepted"}`)
	req := httptest.NewRequest("POST", "/api/v1/terminals/start-composed-session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusPaymentRequired, w.Code,
		"GATING: a composed-session start for a past_due sub beyond the grace period "+
			"must be rejected with 402. Today past_due resolves no plan (RequirePlan "+
			"403s), and no dunning gate exists. Body: %s", w.Body.String())
	assert.True(t, strings.Contains(w.Body.String(), "subscription_past_due"),
		"GATING: the rejection must carry the stable error code `subscription_past_due` "+
			"so the frontend can show a pay-now prompt. Body: %s", w.Body.String())
}
