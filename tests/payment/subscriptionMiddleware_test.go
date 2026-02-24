package payment_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"soli/formations/src/payment/middleware"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockSubscriptionService implements services.UserSubscriptionService for testing
type mockSubscriptionService struct {
	mock.Mock
}

func (m *mockSubscriptionService) HasActiveSubscription(userID string) (bool, error) {
	args := m.Called(userID)
	return args.Bool(0), args.Error(1)
}

func (m *mockSubscriptionService) GetActiveUserSubscription(userID string) (*models.UserSubscription, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *mockSubscriptionService) GetAllActiveUserSubscriptions(userID string) ([]models.UserSubscription, error) {
	args := m.Called(userID)
	return args.Get(0).([]models.UserSubscription), args.Error(1)
}

func (m *mockSubscriptionService) GetPrimaryUserSubscription(userID string) (*models.UserSubscription, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *mockSubscriptionService) GetUserSubscriptionByID(id uuid.UUID) (*models.UserSubscription, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *mockSubscriptionService) CreateUserSubscription(userID string, planID uuid.UUID) (*models.UserSubscription, error) {
	args := m.Called(userID, planID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *mockSubscriptionService) UpgradeUserPlan(userID string, newPlanID uuid.UUID, prorationBehavior string) (*models.UserSubscription, error) {
	args := m.Called(userID, newPlanID, prorationBehavior)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *mockSubscriptionService) CheckUsageLimit(userID, metricType string, increment int64) (*services.UsageLimitCheck, error) {
	args := m.Called(userID, metricType, increment)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.UsageLimitCheck), args.Error(1)
}

func (m *mockSubscriptionService) IncrementUsage(userID, metricType string, increment int64) error {
	args := m.Called(userID, metricType, increment)
	return args.Error(0)
}

func (m *mockSubscriptionService) GetUserUsageMetrics(userID string) (*[]models.UsageMetrics, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.UsageMetrics), args.Error(1)
}

func (m *mockSubscriptionService) ResetMonthlyUsage(userID string) error {
	args := m.Called(userID)
	return args.Error(0)
}

func (m *mockSubscriptionService) UpdateUsageMetricLimits(userID string, newPlanID uuid.UUID) error {
	args := m.Called(userID, newPlanID)
	return args.Error(0)
}

func (m *mockSubscriptionService) InitializeUsageMetrics(userID string, subscriptionID uuid.UUID, planID uuid.UUID) error {
	args := m.Called(userID, subscriptionID, planID)
	return args.Error(0)
}

func (m *mockSubscriptionService) GetUserPaymentMethods(userID string) (*[]models.PaymentMethod, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.PaymentMethod), args.Error(1)
}

func (m *mockSubscriptionService) SetDefaultPaymentMethod(userID string, paymentMethodID uuid.UUID) error {
	args := m.Called(userID, paymentMethodID)
	return args.Error(0)
}

func (m *mockSubscriptionService) GetUserInvoices(userID string) (*[]models.Invoice, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.Invoice), args.Error(1)
}

func (m *mockSubscriptionService) GetInvoiceByID(id uuid.UUID) (*models.Invoice, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Invoice), args.Error(1)
}

func (m *mockSubscriptionService) GetSubscriptionAnalytics() (*services.SubscriptionAnalytics, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.SubscriptionAnalytics), args.Error(1)
}

func (m *mockSubscriptionService) UpdateUserRoleBasedOnSubscription(userID string) error {
	args := m.Called(userID)
	return args.Error(0)
}

func (m *mockSubscriptionService) GetRequiredRoleForPlan(planID uuid.UUID) (string, error) {
	args := m.Called(planID)
	return args.String(0), args.Error(1)
}

func (m *mockSubscriptionService) GetUserBillingAddresses(userID string) (*[]models.BillingAddress, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.BillingAddress), args.Error(1)
}

func (m *mockSubscriptionService) SetDefaultBillingAddress(userID string, addressID uuid.UUID) error {
	args := m.Called(userID, addressID)
	return args.Error(0)
}

func (m *mockSubscriptionService) GetSubscriptionPlan(id uuid.UUID) (*models.SubscriptionPlan, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.SubscriptionPlan), args.Error(1)
}

func (m *mockSubscriptionService) GetAllSubscriptionPlans(activeOnly bool) (*[]models.SubscriptionPlan, error) {
	args := m.Called(activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.SubscriptionPlan), args.Error(1)
}

func (m *mockSubscriptionService) AdminAssignSubscription(userID string, planID uuid.UUID, durationDays int, assignedByUserID string) (*models.UserSubscription, error) {
	args := m.Called(userID, planID, durationDays, assignedByUserID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

// ==========================================
// InjectSubscriptionInfo Middleware Tests
// ==========================================

func TestInjectSubscriptionInfo_NoSubscription_DoesNotPanic(t *testing.T) {
	mockService := new(mockSubscriptionService)

	// User has no active subscription
	mockService.On("GetActiveUserSubscription", "user-no-sub").Return(nil, fmt.Errorf("record not found"))
	mockService.On("GetUserUsageMetrics", "user-no-sub").Return(nil, fmt.Errorf("not found"))

	mw := middleware.NewSubscriptionIntegrationMiddlewareWithService(mockService)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "user-no-sub")
		c.Next()
	})
	router.GET("/test", mw.InjectSubscriptionInfo(), func(c *gin.Context) {
		hasActive := c.GetBool("has_active_subscription")
		assert.False(t, hasActive, "has_active_subscription should be false")
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	// Should NOT panic
	assert.NotPanics(t, func() {
		router.ServeHTTP(w, req)
	})

	assert.Equal(t, http.StatusOK, w.Code, "Should proceed to handler when no subscription")
}

func TestInjectSubscriptionInfo_WithSubscription_InjectsPlan(t *testing.T) {
	mockService := new(mockSubscriptionService)

	planID := uuid.New()
	subscription := &models.UserSubscription{
		UserID:             "user-with-sub",
		SubscriptionPlanID: planID,
		Status:             "active",
	}
	plan := &models.SubscriptionPlan{
		Name:     "Test Plan",
		Features: []string{"feature1"},
	}
	plan.ID = planID

	mockService.On("GetActiveUserSubscription", "user-with-sub").Return(subscription, nil)
	mockService.On("GetSubscriptionPlan", planID).Return(plan, nil)
	mockService.On("GetUserUsageMetrics", "user-with-sub").Return(nil, fmt.Errorf("not found"))

	mw := middleware.NewSubscriptionIntegrationMiddlewareWithService(mockService)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "user-with-sub")
		c.Next()
	})
	router.GET("/test", mw.InjectSubscriptionInfo(), func(c *gin.Context) {
		hasActive := c.GetBool("has_active_subscription")
		assert.True(t, hasActive, "has_active_subscription should be true")

		planInterface, exists := c.Get("subscription_plan")
		assert.True(t, exists, "subscription_plan should be in context")
		assert.NotNil(t, planInterface, "subscription_plan should not be nil")

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestInjectSubscriptionInfo_WithSubscription_InvalidPlan_Returns403(t *testing.T) {
	mockService := new(mockSubscriptionService)

	planID := uuid.New()
	subscription := &models.UserSubscription{
		UserID:             "user-bad-plan",
		SubscriptionPlanID: planID,
		Status:             "active",
	}

	mockService.On("GetActiveUserSubscription", "user-bad-plan").Return(subscription, nil)
	mockService.On("GetSubscriptionPlan", planID).Return(nil, fmt.Errorf("plan not found"))

	mw := middleware.NewSubscriptionIntegrationMiddlewareWithService(mockService)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "user-bad-plan")
		c.Next()
	})
	router.GET("/test", mw.InjectSubscriptionInfo(), func(c *gin.Context) {
		t.Fatal("Handler should not be reached")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "Should return 403 when subscription exists but plan is invalid")
}

func TestInjectSubscriptionInfo_NoUserId_Proceeds(t *testing.T) {
	mockService := new(mockSubscriptionService)

	mw := middleware.NewSubscriptionIntegrationMiddlewareWithService(mockService)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	// No userId set in context
	router.GET("/test", mw.InjectSubscriptionInfo(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should proceed when no userId in context")
}
