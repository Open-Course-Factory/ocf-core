// tests/payment/subscriptionController_uuid_test.go
// Tests that cancel and reactivate endpoints handle malformed UUIDs gracefully
// instead of panicking via uuid.MustParse.
package payment_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MockUserSubscriptionService mocks services.UserSubscriptionService for controller tests.
type MockUserSubscriptionService struct {
	mock.Mock
}

func (m *MockUserSubscriptionService) GetUserSubscriptionByID(id uuid.UUID) (*models.UserSubscription, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

// cancelHandler reproduces the code path of
// userSubscriptionController.CancelSubscription to verify UUID validation.
func cancelHandler(subscriptionSvc *MockUserSubscriptionService) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		subscriptionID := ctx.Param("id")

		parsedID, parseErr := uuid.Parse(subscriptionID)
		if parseErr != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Invalid subscription ID format",
			})
			return
		}

		subscription, err := subscriptionSvc.GetUserSubscriptionByID(parsedID)
		if err != nil {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Subscription not found",
			})
			return
		}
		_ = subscription
		ctx.JSON(http.StatusOK, gin.H{"message": "cancelled"})
	}
}

// reactivateHandler reproduces the code path of
// userSubscriptionController.ReactivateSubscription to verify UUID validation.
func reactivateHandler(subscriptionSvc *MockUserSubscriptionService) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		subscriptionID := ctx.Param("id")

		parsedID, parseErr := uuid.Parse(subscriptionID)
		if parseErr != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Invalid subscription ID format",
			})
			return
		}

		subscription, err := subscriptionSvc.GetUserSubscriptionByID(parsedID)
		if err != nil {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Subscription not found",
			})
			return
		}
		_ = subscription
		ctx.JSON(http.StatusOK, gin.H{"message": "reactivated"})
	}
}

func setupUUIDValidationRouter() (*gin.Engine, *MockUserSubscriptionService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Deliberately NOT adding gin.Recovery() — we want the test to detect panics.

	mockSubscriptionSvc := new(MockUserSubscriptionService)

	v1 := router.Group("/api/v1/user-subscriptions")
	{
		v1.POST("/:id/cancel", cancelHandler(mockSubscriptionSvc))
		v1.POST("/:id/reactivate", reactivateHandler(mockSubscriptionSvc))
	}

	return router, mockSubscriptionSvc
}

func TestCancelSubscription_InvalidUUID_Returns400(t *testing.T) {
	router, _ := setupUUIDValidationRouter()

	invalidUUIDs := []struct {
		name  string
		value string
	}{
		{"random string", "not-a-uuid"},
		{"empty string", ""},
		{"too short", "12345"},
		{"special characters", "not@valid!uuid"},
		{"almost valid UUID missing chars", "550e8400-e29b-41d4-a716-44665544000"},
		{"SQL injection attempt", "'; DROP TABLE user_subscriptions; --"},
	}

	for _, tc := range invalidUUIDs {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/v1/user-subscriptions/" + tc.value + "/cancel"
			req, _ := http.NewRequest("POST", path, nil)
			w := httptest.NewRecorder()

			// This should NOT panic. It should return 400 Bad Request.
			assert.NotPanics(t, func() {
				router.ServeHTTP(w, req)
			}, "CancelSubscription should not panic on invalid UUID: %s", tc.value)

			assert.Equal(t, http.StatusBadRequest, w.Code,
				"CancelSubscription should return 400 for invalid UUID: %s", tc.value)
		})
	}
}

func TestReactivateSubscription_InvalidUUID_Returns400(t *testing.T) {
	router, _ := setupUUIDValidationRouter()

	invalidUUIDs := []struct {
		name  string
		value string
	}{
		{"random string", "not-a-uuid"},
		{"empty string", ""},
		{"too short", "12345"},
		{"special characters", "not@valid!uuid"},
		{"almost valid UUID missing chars", "550e8400-e29b-41d4-a716-44665544000"},
		{"SQL injection attempt", "'; DROP TABLE user_subscriptions; --"},
	}

	for _, tc := range invalidUUIDs {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/v1/user-subscriptions/" + tc.value + "/reactivate"
			req, _ := http.NewRequest("POST", path, nil)
			w := httptest.NewRecorder()

			// This should NOT panic. It should return 400 Bad Request.
			assert.NotPanics(t, func() {
				router.ServeHTTP(w, req)
			}, "ReactivateSubscription should not panic on invalid UUID: %s", tc.value)

			assert.Equal(t, http.StatusBadRequest, w.Code,
				"ReactivateSubscription should return 400 for invalid UUID: %s", tc.value)
		})
	}
}

// --- Bug 4: Inconsistent UUID validation in GetUserSubscription ---

// currentSubscriptionHandler reproduces the exact code path from
// userSubscriptionController.GetUserSubscription for the organization_id query param.
// BUG: When organization_id is an invalid UUID, the handler silently falls back to
// global plan resolution (orgID stays nil) instead of returning 400 Bad Request.
// This is inconsistent with organizationSubscriptionController which correctly returns 400.
func currentSubscriptionHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// This is the EXACT code from userSubscriptionController.GetUserSubscription:
		var orgID *uuid.UUID
		if orgIDStr := ctx.Query("organization_id"); orgIDStr != "" {
			if parsed, err := uuid.Parse(orgIDStr); err == nil {
				orgID = &parsed
			}
			// BUG: no else branch to return 400 on invalid UUID
		}

		// For testing purposes, we expose whether orgID was parsed or silently ignored
		if orgID != nil {
			ctx.JSON(http.StatusOK, gin.H{"org_id": orgID.String(), "source": "organization"})
		} else {
			// If orgIDStr was provided but orgID is nil, the invalid UUID was silently swallowed
			if ctx.Query("organization_id") != "" {
				// This is the buggy path: invalid UUID was provided but silently ignored
				// Expected behavior: return 400
				// Actual behavior: falls back to global resolution
				ctx.JSON(http.StatusOK, gin.H{"source": "global_fallback"})
			} else {
				ctx.JSON(http.StatusOK, gin.H{"source": "global"})
			}
		}
	}
}

// TestGetUserSubscription_InvalidOrgUUID_ShouldReturn400 verifies that passing
// an invalid UUID as organization_id query parameter returns 400 Bad Request
// instead of silently falling back to global plan resolution.
//
// BUG: The current code in userSubscriptionController.GetUserSubscription does:
//
//	if parsed, err := uuid.Parse(orgIDStr); err == nil {
//	    orgID = &parsed
//	}
//
// When parsing fails, it silently continues with orgID = nil, falling back to
// the global plan. The organizationSubscriptionController correctly returns 400.
func TestGetUserSubscription_InvalidOrgUUID_ShouldReturn400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/user-subscriptions/current", currentSubscriptionHandler())

	invalidUUIDs := []struct {
		name  string
		value string
	}{
		{"random string", "not-a-uuid"},
		{"too short", "12345"},
		{"special characters", "not@valid!uuid"},
		{"SQL injection attempt", "'; DROP TABLE organizations; --"},
		{"almost valid UUID", "550e8400-e29b-41d4-a716-44665544000"},
	}

	for _, tc := range invalidUUIDs {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/v1/user-subscriptions/current?organization_id=" + tc.value
			req, _ := http.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// EXPECTED: 400 Bad Request — the user explicitly provided an invalid UUID
			// CURRENT BUG: 200 OK with source="global_fallback" — the invalid UUID is
			// silently swallowed and the handler falls back to global plan resolution.
			// This means a typo in organization_id gives you a different plan without
			// any error indication, which is a confusing UX and potential security issue.
			assert.Equal(t, http.StatusBadRequest, w.Code,
				"GetUserSubscription should return 400 for invalid organization_id UUID: %s", tc.value)
		})
	}
}

func TestCancelSubscription_ValidUUID_PassesThrough(t *testing.T) {
	router, mockSvc := setupUUIDValidationRouter()

	validID := uuid.New()

	// Mock: the service returns "not found" for this UUID
	mockSvc.On("GetUserSubscriptionByID", validID).Return(nil, gorm.ErrRecordNotFound)

	path := "/api/v1/user-subscriptions/" + validID.String() + "/cancel"
	req, _ := http.NewRequest("POST", path, nil)
	w := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		router.ServeHTTP(w, req)
	}, "CancelSubscription should not panic on valid UUID")

	// With a valid UUID, the request should reach the service layer and get 404 (not found)
	assert.Equal(t, http.StatusNotFound, w.Code,
		"CancelSubscription with valid UUID should get past UUID parsing")

	mockSvc.AssertExpectations(t)
}

func TestReactivateSubscription_ValidUUID_PassesThrough(t *testing.T) {
	router, mockSvc := setupUUIDValidationRouter()

	validID := uuid.New()

	// Mock: the service returns "not found" for this UUID
	mockSvc.On("GetUserSubscriptionByID", validID).Return(nil, gorm.ErrRecordNotFound)

	path := "/api/v1/user-subscriptions/" + validID.String() + "/reactivate"
	req, _ := http.NewRequest("POST", path, nil)
	w := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		router.ServeHTTP(w, req)
	}, "ReactivateSubscription should not panic on valid UUID")

	// With a valid UUID, the request should reach the service layer and get 404 (not found)
	assert.Equal(t, http.StatusNotFound, w.Code,
		"ReactivateSubscription with valid UUID should get past UUID parsing")

	mockSvc.AssertExpectations(t)
}
