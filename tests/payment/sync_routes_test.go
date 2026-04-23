// tests/payment/sync_routes_test.go
package payment_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func setupSyncTestRouter() (*gin.Engine, *SharedMockStripeService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	mockStripeService := new(SharedMockStripeService)

	// Setup routes for testing
	v1 := router.Group("/api/v1")
	{
		v1.POST("/user-subscriptions/sync-existing", func(c *gin.Context) {
			result, err := mockStripeService.SyncExistingSubscriptions()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, result)
		})

		v1.POST("/user-subscriptions/users/:user_id/sync", func(c *gin.Context) {
			userID := c.Param("user_id")
			result, err := mockStripeService.SyncUserSubscriptions(userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, result)
		})

		v1.POST("/user-subscriptions/sync-missing-metadata", func(c *gin.Context) {
			result, err := mockStripeService.SyncSubscriptionsWithMissingMetadata()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, result)
		})

		v1.POST("/user-subscriptions/link/:subscription_id", func(c *gin.Context) {
			subscriptionID := c.Param("subscription_id")
			var request struct {
				UserID             string    `json:"user_id" binding:"required"`
				SubscriptionPlanID uuid.UUID `json:"subscription_plan_id" binding:"required"`
			}

			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			err := mockStripeService.LinkSubscriptionToUser(subscriptionID, request.UserID, request.SubscriptionPlanID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "Subscription linked successfully"})
		})
	}

	return router, mockStripeService
}

func TestSyncRoutes_SyncExistingSubscriptions(t *testing.T) {
	router, mockStripeService := setupSyncTestRouter()

	t.Run("Successful sync of existing subscriptions", func(t *testing.T) {
		expectedResult := &services.SyncSubscriptionsResult{
			ProcessedSubscriptions: 5,
			CreatedSubscriptions:   3,
			UpdatedSubscriptions:   1,
			SkippedSubscriptions:   1,
			FailedSubscriptions:    []services.FailedSubscription{},
			CreatedDetails: []string{
				"Created subscription sub_new1 for user_123",
				"Created subscription sub_new2 for user_456",
				"Created subscription sub_new3 for user_789",
			},
			UpdatedDetails: []string{
				"Updated subscription sub_existing for user_101",
			},
			SkippedDetails: []string{
				"Skipped subscription sub_skip: already exists",
			},
		}

		mockStripeService.On("SyncExistingSubscriptions").Return(expectedResult, nil)

		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/sync-existing", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var result services.SyncSubscriptionsResult
		err := json.Unmarshal(w.Body.Bytes(), &result)
		assert.NoError(t, err)

		assert.Equal(t, 5, result.ProcessedSubscriptions)
		assert.Equal(t, 3, result.CreatedSubscriptions)
		assert.Equal(t, 1, result.UpdatedSubscriptions)
		assert.Equal(t, 1, result.SkippedSubscriptions)
		assert.Len(t, result.FailedSubscriptions, 0)
		assert.Len(t, result.CreatedDetails, 3)
		assert.Len(t, result.UpdatedDetails, 1)
		assert.Len(t, result.SkippedDetails, 1)

		mockStripeService.AssertExpectations(t)
	})

	t.Run("Sync with failures", func(t *testing.T) {
		router, mockStripeService := setupSyncTestRouter() // Fresh router for this test
		expectedResult := &services.SyncSubscriptionsResult{
			ProcessedSubscriptions: 3,
			CreatedSubscriptions:   1,
			UpdatedSubscriptions:   0,
			SkippedSubscriptions:   1,
			FailedSubscriptions: []services.FailedSubscription{
				{
					StripeSubscriptionID: "sub_failed",
					UserID:               "",
					Error:                "Missing metadata",
				},
			},
			CreatedDetails: []string{"Created subscription sub_success for user_123"},
			UpdatedDetails: []string{},
			SkippedDetails: []string{"Skipped subscription sub_skip: invalid data"},
		}

		mockStripeService.On("SyncExistingSubscriptions").Return(expectedResult, nil)

		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/sync-existing", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var result services.SyncSubscriptionsResult
		err := json.Unmarshal(w.Body.Bytes(), &result)
		assert.NoError(t, err)

		assert.Equal(t, 3, result.ProcessedSubscriptions)
		assert.Len(t, result.FailedSubscriptions, 1)
		assert.Equal(t, "sub_failed", result.FailedSubscriptions[0].StripeSubscriptionID)
		assert.Equal(t, "Missing metadata", result.FailedSubscriptions[0].Error)

		mockStripeService.AssertExpectations(t)
	})
}

func TestSyncRoutes_SyncUserSubscriptions(t *testing.T) {
	router, mockStripeService := setupSyncTestRouter()

	t.Run("Successful user-specific sync", func(t *testing.T) {
		userID := "user_123"
		expectedResult := &services.SyncSubscriptionsResult{
			ProcessedSubscriptions: 2,
			CreatedSubscriptions:   1,
			UpdatedSubscriptions:   1,
			SkippedSubscriptions:   0,
			FailedSubscriptions:    []services.FailedSubscription{},
			CreatedDetails:         []string{"Created subscription sub_new for user_123"},
			UpdatedDetails:         []string{"Updated subscription sub_existing for user_123"},
			SkippedDetails:         []string{},
		}

		mockStripeService.On("SyncUserSubscriptions", userID).Return(expectedResult, nil)

		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/users/user_123/sync", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var result services.SyncSubscriptionsResult
		err := json.Unmarshal(w.Body.Bytes(), &result)
		assert.NoError(t, err)

		assert.Equal(t, 2, result.ProcessedSubscriptions)
		assert.Equal(t, 1, result.CreatedSubscriptions)
		assert.Equal(t, 1, result.UpdatedSubscriptions)
		assert.Contains(t, result.CreatedDetails[0], "user_123")
		assert.Contains(t, result.UpdatedDetails[0], "user_123")

		mockStripeService.AssertExpectations(t)
	})

	t.Run("User with no subscriptions", func(t *testing.T) {
		userID := "user_no_subs"
		expectedResult := &services.SyncSubscriptionsResult{
			ProcessedSubscriptions: 0,
			CreatedSubscriptions:   0,
			UpdatedSubscriptions:   0,
			SkippedSubscriptions:   0,
			FailedSubscriptions:    []services.FailedSubscription{},
			CreatedDetails:         []string{},
			UpdatedDetails:         []string{},
			SkippedDetails:         []string{},
		}

		mockStripeService.On("SyncUserSubscriptions", userID).Return(expectedResult, nil)

		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/users/user_no_subs/sync", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var result services.SyncSubscriptionsResult
		err := json.Unmarshal(w.Body.Bytes(), &result)
		assert.NoError(t, err)

		assert.Equal(t, 0, result.ProcessedSubscriptions)
		assert.Empty(t, result.CreatedDetails)
		assert.Empty(t, result.UpdatedDetails)
		assert.Empty(t, result.SkippedDetails)

		mockStripeService.AssertExpectations(t)
	})
}

func TestSyncRoutes_SyncMissingMetadata(t *testing.T) {
	router, mockStripeService := setupSyncTestRouter()

	t.Run("Successful metadata recovery", func(t *testing.T) {
		expectedResult := &services.SyncSubscriptionsResult{
			ProcessedSubscriptions: 3,
			CreatedSubscriptions:   0,
			UpdatedSubscriptions:   2,
			SkippedSubscriptions:   0,
			FailedSubscriptions: []services.FailedSubscription{
				{
					StripeSubscriptionID: "sub_orphaned",
					UserID:               "",
					Error:                "No checkout session found for metadata recovery",
				},
			},
			CreatedDetails: []string{},
			UpdatedDetails: []string{
				"Updated subscription sub_recovered1 with metadata from checkout session",
				"Updated subscription sub_recovered2 with metadata from checkout session",
			},
			SkippedDetails: []string{},
		}

		mockStripeService.On("SyncSubscriptionsWithMissingMetadata").Return(expectedResult, nil)

		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/sync-missing-metadata", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var result services.SyncSubscriptionsResult
		err := json.Unmarshal(w.Body.Bytes(), &result)
		assert.NoError(t, err)

		assert.Equal(t, 3, result.ProcessedSubscriptions)
		assert.Equal(t, 0, result.CreatedSubscriptions)
		assert.Equal(t, 2, result.UpdatedSubscriptions)
		assert.Len(t, result.FailedSubscriptions, 1)
		assert.Equal(t, "sub_orphaned", result.FailedSubscriptions[0].StripeSubscriptionID)
		assert.Contains(t, result.FailedSubscriptions[0].Error, "No checkout session found")

		mockStripeService.AssertExpectations(t)
	})
}

func TestSyncRoutes_LinkSubscriptionToUser(t *testing.T) {
	router, mockStripeService := setupSyncTestRouter()

	t.Run("Successful manual linking", func(t *testing.T) {
		subscriptionID := "sub_manual_link"
		userID := "user_123"
		planID := uuid.New()

		linkRequest := map[string]any{
			"user_id":              userID,
			"subscription_plan_id": planID,
		}

		mockStripeService.On("LinkSubscriptionToUser", subscriptionID, userID, planID).Return(nil)

		requestBody, _ := json.Marshal(linkRequest)
		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/link/"+subscriptionID, bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "Subscription linked successfully", response["message"])

		mockStripeService.AssertExpectations(t)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		subscriptionID := "sub_invalid_request"

		invalidRequest := map[string]any{
			"user_id": "user_123",
			// subscription_plan_id manquant
		}

		requestBody, _ := json.Marshal(invalidRequest)
		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/link/"+subscriptionID, bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["error"], "SubscriptionPlanID")
	})

	t.Run("Empty user ID", func(t *testing.T) {
		subscriptionID := "sub_empty_user"
		planID := uuid.New()

		linkRequest := map[string]any{
			"user_id":              "",
			"subscription_plan_id": planID,
		}

		requestBody, _ := json.Marshal(linkRequest)
		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/link/"+subscriptionID, bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid UUID for subscription plan", func(t *testing.T) {
		subscriptionID := "sub_invalid_uuid"

		linkRequest := map[string]any{
			"user_id":              "user_123",
			"subscription_plan_id": "not-a-valid-uuid",
		}

		requestBody, _ := json.Marshal(linkRequest)
		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/link/"+subscriptionID, bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestSyncRoutes_ErrorHandling(t *testing.T) {
	router, mockStripeService := setupSyncTestRouter()

	t.Run("Service error during sync", func(t *testing.T) {
		expectedError := assert.AnError
		mockStripeService.On("SyncExistingSubscriptions").Return((*services.SyncSubscriptionsResult)(nil), expectedError)

		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/sync-existing", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["error"], "assert.AnError")

		mockStripeService.AssertExpectations(t)
	})

	t.Run("Service error during user sync", func(t *testing.T) {
		userID := "user_error"
		expectedError := assert.AnError
		mockStripeService.On("SyncUserSubscriptions", userID).Return((*services.SyncSubscriptionsResult)(nil), expectedError)

		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/users/"+userID+"/sync", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		mockStripeService.AssertExpectations(t)
	})

	t.Run("Service error during metadata sync", func(t *testing.T) {
		expectedError := assert.AnError
		mockStripeService.On("SyncSubscriptionsWithMissingMetadata").Return((*services.SyncSubscriptionsResult)(nil), expectedError)

		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/sync-missing-metadata", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		mockStripeService.AssertExpectations(t)
	})

	t.Run("Service error during manual linking", func(t *testing.T) {
		subscriptionID := "sub_link_error"
		userID := "user_123"
		planID := uuid.New()

		linkRequest := map[string]any{
			"user_id":              userID,
			"subscription_plan_id": planID,
		}

		expectedError := assert.AnError
		mockStripeService.On("LinkSubscriptionToUser", subscriptionID, userID, planID).Return(expectedError)

		requestBody, _ := json.Marshal(linkRequest)
		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/link/"+subscriptionID, bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		mockStripeService.AssertExpectations(t)
	})
}

func TestSyncRoutes_BatchOperations(t *testing.T) {
	router, mockStripeService := setupSyncTestRouter()

	t.Run("Large batch sync operation", func(t *testing.T) {
		// Simuler un sync de grande quantité
		createdDetails := make([]string, 50)
		for i := 0; i < 50; i++ {
			createdDetails[i] = "Created subscription sub_batch_" + string(rune(i+1)) + " for user_batch_" + string(rune(i+1))
		}

		expectedResult := &services.SyncSubscriptionsResult{
			ProcessedSubscriptions: 100,
			CreatedSubscriptions:   50,
			UpdatedSubscriptions:   30,
			SkippedSubscriptions:   15,
			FailedSubscriptions: []services.FailedSubscription{
				{StripeSubscriptionID: "sub_fail1", Error: "Invalid metadata"},
				{StripeSubscriptionID: "sub_fail2", Error: "User not found"},
				{StripeSubscriptionID: "sub_fail3", Error: "Plan not found"},
				{StripeSubscriptionID: "sub_fail4", Error: "Already exists"},
				{StripeSubscriptionID: "sub_fail5", Error: "Payment method required"},
			},
			CreatedDetails: createdDetails[:10], // Limiter pour le test
			UpdatedDetails: []string{
				"Updated subscription sub_update1",
				"Updated subscription sub_update2",
			},
			SkippedDetails: []string{
				"Skipped subscription sub_skip1: already exists",
				"Skipped subscription sub_skip2: inactive",
			},
		}

		mockStripeService.On("SyncExistingSubscriptions").Return(expectedResult, nil)

		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/sync-existing", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var result services.SyncSubscriptionsResult
		err := json.Unmarshal(w.Body.Bytes(), &result)
		assert.NoError(t, err)

		assert.Equal(t, 100, result.ProcessedSubscriptions)
		assert.Equal(t, 50, result.CreatedSubscriptions)
		assert.Equal(t, 30, result.UpdatedSubscriptions)
		assert.Equal(t, 15, result.SkippedSubscriptions)
		assert.Len(t, result.FailedSubscriptions, 5)

		// Vérifier que les totaux correspondent
		totalAccounted := result.CreatedSubscriptions + result.UpdatedSubscriptions + result.SkippedSubscriptions + len(result.FailedSubscriptions)
		assert.Equal(t, result.ProcessedSubscriptions, totalAccounted)

		mockStripeService.AssertExpectations(t)
	})
}

// ============================================================================
// Issue #262 — Trial plan must not be pushed to Stripe on /sync-stripe
//
// The Trial plan (PriceAmount == 0) is intentionally decoupled from Stripe:
// it is auto-assigned to every new user/org and has no billing lifecycle.
// The POST /subscription-plans/sync-stripe endpoint must therefore skip
// free plans rather than creating a bogus €0/month recurring price product
// in Stripe.
//
// These tests mirror the structure of setupSyncTestRouter above, replicating
// the SyncAllSubscriptionPlansWithStripe handler inline (same pattern as the
// existing sync-routes tests — the real userSubscriptionController struct is
// unexported and cannot be instantiated from the test package).
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
//        and the response lists Trial under skipped_plans (not synced_plans).
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
