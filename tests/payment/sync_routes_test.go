// tests/payment/sync_routes_test.go
package payment_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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

		linkRequest := map[string]interface{}{
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

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "Subscription linked successfully", response["message"])

		mockStripeService.AssertExpectations(t)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		subscriptionID := "sub_invalid_request"

		invalidRequest := map[string]interface{}{
			"user_id": "user_123",
			// subscription_plan_id manquant
		}

		requestBody, _ := json.Marshal(invalidRequest)
		req := httptest.NewRequest("POST", "/api/v1/user-subscriptions/link/"+subscriptionID, bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["error"], "SubscriptionPlanID")
	})

	t.Run("Empty user ID", func(t *testing.T) {
		subscriptionID := "sub_empty_user"
		planID := uuid.New()

		linkRequest := map[string]interface{}{
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

		linkRequest := map[string]interface{}{
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

		var response map[string]interface{}
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

		linkRequest := map[string]interface{}{
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
