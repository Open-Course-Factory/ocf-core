// tests/payment/sync_routes_test.go
package payment_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ============================================================================
// Issue #262 — Trial plan must not be pushed to Stripe on /sync-stripe
//
// The Trial plan (PriceAmount == 0) is intentionally decoupled from Stripe:
// it is auto-assigned to every new user/org and has no billing lifecycle.
// The POST /subscription-plans/sync-stripe endpoint must therefore skip
// free plans rather than creating a bogus €0/month recurring price product
// in Stripe.
//
// These tests replicate the SyncAllSubscriptionPlansWithStripe handler inline
// (the real userSubscriptionController struct is unexported and cannot be
// instantiated from the test package).
// ============================================================================

// setupSyncPlansRouter builds a gin router wired to the mock subscription +
// stripe services and installed the handler under test at
// POST /api/v1/subscription-plans/sync-stripe. The handler body mirrors
// src/payment/routes/userSubscriptionController.go:SyncAllSubscriptionPlansWithStripe.
func setupSyncPlansRouter() (*gin.Engine, *SharedMockSubscriptionService, *SharedMockStripeService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	mockSubscriptionService := new(SharedMockSubscriptionService)
	mockStripeService := new(SharedMockStripeService)

	v1 := router.Group("/api/v1")
	{
		v1.POST("/subscription-plans/sync-stripe", func(c *gin.Context) {
			plansPtr, err := mockSubscriptionService.GetAllSubscriptionPlans(false)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			plans := *plansPtr
			var syncedPlans []string
			var skippedPlans []string
			var failedPlans []map[string]any

			for _, plan := range plans {
				if plan.StripePriceID != nil {
					skippedPlans = append(skippedPlans, plan.Name+" (already synced)")
					continue
				}

				if plan.PriceAmount == 0 {
					skippedPlans = append(skippedPlans, plan.Name+" (free plan)")
					continue
				}

				err := mockStripeService.CreateSubscriptionPlanInStripe(&plan)
				if err != nil {
					failedPlans = append(failedPlans, map[string]any{
						"name":  plan.Name,
						"id":    plan.ID.String(),
						"error": err.Error(),
					})
				} else {
					syncedPlans = append(syncedPlans, plan.Name)
				}
			}

			c.JSON(http.StatusOK, map[string]any{
				"synced_plans":  syncedPlans,
				"skipped_plans": skippedPlans,
				"failed_plans":  failedPlans,
				"total_plans":   len(plans),
			})
		})
	}

	return router, mockSubscriptionService, mockStripeService
}

// TestSyncAllSubscriptionPlans_TrialPlanIsSkipped — issue #262.
// Given: DB contains Trial (price=0), Member Pro (price=1200), Trainer Plan (price=1200).
// When : POST /api/v1/subscription-plans/sync-stripe is called.
// Then : CreateSubscriptionPlanInStripe is invoked EXACTLY for the two paid plans
//
//	and the response lists Trial under skipped_plans (not synced_plans).
func TestSyncAllSubscriptionPlans_TrialPlanIsSkipped(t *testing.T) {
	router, mockSubscriptionService, mockStripeService := setupSyncPlansRouter()

	trialPlan := models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Trial",
		Description:     "Free plan",
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	memberProPlan := models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Member Pro",
		Description:     "Paid member plan",
		PriceAmount:     1200,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	trainerPlan := models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Trainer Plan",
		Description:     "Paid trainer plan",
		PriceAmount:     1200,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}
	allPlans := []models.SubscriptionPlan{trialPlan, memberProPlan, trainerPlan}

	mockSubscriptionService.On("GetAllSubscriptionPlans", false).Return(&allPlans, nil)

	// The stripe call must happen ONLY for plans with PriceAmount > 0.
	// Using MatchedBy with PriceAmount > 0 — if the handler calls the mock
	// for the Trial plan (PriceAmount == 0), testify will fail the test
	// with an "unexpected method call" panic because no matcher covers it.
	mockStripeService.
		On("CreateSubscriptionPlanInStripe", mock.MatchedBy(func(p any) bool {
			plan, ok := p.(*models.SubscriptionPlan)
			return ok && plan.PriceAmount > 0
		})).
		Return(nil).
		Times(2)

	req := httptest.NewRequest("POST", "/api/v1/subscription-plans/sync-stripe", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "sync endpoint must succeed")

	var response map[string]any
	require := assert.New(t)
	require.NoError(json.Unmarshal(w.Body.Bytes(), &response))

	synced, _ := response["synced_plans"].([]any)
	skipped, _ := response["skipped_plans"].([]any)

	syncedNames := make([]string, 0, len(synced))
	for _, name := range synced {
		syncedNames = append(syncedNames, name.(string))
	}
	skippedJoined := ""
	for _, entry := range skipped {
		skippedJoined += entry.(string) + "|"
	}

	// Trial must NOT be synced.
	assert.NotContains(t, syncedNames, "Trial",
		"Trial plan (price=0) must not be pushed to Stripe: it is decoupled from billing")

	// Paid plans must be synced.
	assert.Contains(t, syncedNames, "Member Pro", "paid plan must be synced")
	assert.Contains(t, syncedNames, "Trainer Plan", "paid plan must be synced")

	// Trial should appear in skipped_plans so operators see it was intentionally left out.
	assert.Contains(t, skippedJoined, "Trial",
		"Trial plan should be reported in skipped_plans (not silently dropped)")

	// Exactly 2 stripe calls were expected — will fail if a 3rd call was made for Trial.
	mockStripeService.AssertExpectations(t)
	mockSubscriptionService.AssertExpectations(t)
}

// TestSyncAllSubscriptionPlans_FreePlanNotSentToStripe tightens the contract:
// regardless of plan name, PriceAmount == 0 must never trigger a Stripe call.
// This guards against a future "free tier" plan being added with a non-"Trial"
// name and silently leaking to Stripe.
func TestSyncAllSubscriptionPlans_FreePlanNotSentToStripe(t *testing.T) {
	router, mockSubscriptionService, mockStripeService := setupSyncPlansRouter()

	freePlans := []models.SubscriptionPlan{
		{
			BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
			Name:            "Community Free",
			PriceAmount:     0,
			Currency:        "eur",
			BillingInterval: "month",
			IsActive:        true,
		},
		{
			BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
			Name:            "Starter Zero",
			PriceAmount:     0,
			Currency:        "eur",
			BillingInterval: "month",
			IsActive:        true,
		},
	}

	mockSubscriptionService.On("GetAllSubscriptionPlans", false).Return(&freePlans, nil)
	// No .On for CreateSubscriptionPlanInStripe — any call at all must fail the test.

	req := httptest.NewRequest("POST", "/api/v1/subscription-plans/sync-stripe", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require := assert.New(t)
	require.NoError(json.Unmarshal(w.Body.Bytes(), &response))

	synced, _ := response["synced_plans"].([]any)
	assert.Empty(t, synced, "free plans must never appear in synced_plans")

	// Mock must NOT have been called at all.
	mockStripeService.AssertNotCalled(t, "CreateSubscriptionPlanInStripe", mock.Anything)
	mockSubscriptionService.AssertExpectations(t)
}

// ============================================================================
// Issue #262 (single-plan path) — POST /subscription-plans/:id/sync-stripe
//
// MR !178 guarded the sync-ALL path against free plans, but the single-plan
// endpoint SyncSubscriptionPlanWithStripe (userSubscriptionController.go) has
// NO free-plan guard: calling it on a free plan (PriceAmount == 0) currently
// pushes a bogus €0/month product to Stripe (or returns 200-success leaving
// StripePriceID nil). The #262 intent applies here too — a free plan must be
// rejected, NOT silently "synced".
//
// This router mirrors the CURRENT single-plan handler logic inline (the real
// controller struct is unexported, same constraint as setupSyncPlansRouter).
// The handler body intentionally reproduces today's branches — including the
// MISSING free-plan guard — so the test below fails (RED) until backend-dev
// adds the guard to both the real handler and this replica.
//
// Contract asserted (matches the handler's existing "already has a Stripe
// price" branch, which returns HTTP 400): a free plan must yield HTTP 400 with
// a "free plan" message, and CreateSubscriptionPlanInStripe must NOT be called.
// ============================================================================

// setupSyncSinglePlanRouter wires the mock subscription + stripe services to a
// handler installed at POST /api/v1/subscription-plans/:id/sync-stripe. The
// handler body mirrors userSubscriptionController.SyncSubscriptionPlanWithStripe
// as it exists today (no free-plan guard).
func setupSyncSinglePlanRouter() (*gin.Engine, *SharedMockSubscriptionService, *SharedMockStripeService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	mockSubscriptionService := new(SharedMockSubscriptionService)
	mockStripeService := new(SharedMockStripeService)

	v1 := router.Group("/api/v1")
	{
		v1.POST("/subscription-plans/:id/sync-stripe", func(c *gin.Context) {
			planID, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error_message": "Invalid plan ID"})
				return
			}

			plan, err := mockSubscriptionService.GetSubscriptionPlan(planID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error_message": "Subscription plan not found"})
				return
			}

			if plan.StripePriceID != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error_message": "Plan already has a Stripe price configured"})
				return
			}

			// Mirrors the real handler: free plans are decoupled from Stripe
			// and must be rejected rather than pushed as a bogus €0/month price.
			if plan.IsFree() {
				c.JSON(http.StatusBadRequest, gin.H{"error_message": "Cannot sync a free plan with Stripe: free plans are decoupled from billing"})
				return
			}

			if err := mockStripeService.CreateSubscriptionPlanInStripe(plan); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error_message": "Failed to sync plan with Stripe: " + err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"name": plan.Name})
		})
	}

	return router, mockSubscriptionService, mockStripeService
}

// TestSyncSubscriptionPlan_FreePlan_NotSyncedAndRejected — issue #262.
// Given: a free plan (PriceAmount == 0, no StripePriceID).
// When : POST /api/v1/subscription-plans/{id}/sync-stripe is called for it.
// Then : the endpoint rejects with HTTP 400 ("free plan") and NEVER calls
//
//	CreateSubscriptionPlanInStripe — the free plan stays decoupled.
func TestSyncSubscriptionPlan_FreePlan_NotSyncedAndRejected(t *testing.T) {
	router, mockSubscriptionService, mockStripeService := setupSyncSinglePlanRouter()

	freePlanID := uuid.New()
	freePlan := models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: freePlanID},
		Name:            "Trial",
		Description:     "Free plan",
		PriceAmount:     0,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		// StripePriceID nil — never synced.
	}

	mockSubscriptionService.On("GetSubscriptionPlan", freePlanID).Return(&freePlan, nil)

	// Only a PriceAmount > 0 plan may ever be sent to Stripe. If the handler
	// calls the mock for this free plan, testify fails with an unexpected-call
	// panic because no matcher covers PriceAmount == 0.
	mockStripeService.
		On("CreateSubscriptionPlanInStripe", mock.MatchedBy(func(p any) bool {
			plan, ok := p.(*models.SubscriptionPlan)
			return ok && plan.PriceAmount > 0
		})).
		Return(nil)

	req := httptest.NewRequest("POST", "/api/v1/subscription-plans/"+freePlanID.String()+"/sync-stripe", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The fix must distinguish "not syncable" from a successful sync. Mirror
	// the handler's existing "already has a Stripe price" branch → HTTP 400.
	assert.Equal(t, http.StatusBadRequest, w.Code,
		"syncing a free plan must be rejected, not returned as a 200 success")

	var response map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Contains(t, response, "error_message",
		"a rejected free-plan sync must return an error_message, not a plan DTO")
	if msg, ok := response["error_message"].(string); ok {
		assert.Contains(t, msg, "free",
			"the rejection must explain the plan is free (decoupled from Stripe)")
	}

	// The free plan must never be pushed to Stripe.
	mockStripeService.AssertNotCalled(t, "CreateSubscriptionPlanInStripe", mock.Anything)
	mockSubscriptionService.AssertExpectations(t)
}
