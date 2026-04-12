package payment_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	paymentMiddleware "soli/formations/src/payment/middleware"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockEffectivePlanService implements services.EffectivePlanService for unit testing.
type mockEffectivePlanService struct {
	mock.Mock
}

func (m *mockEffectivePlanService) GetUserEffectivePlan(userID string) (*paymentServices.EffectivePlanResult, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*paymentServices.EffectivePlanResult), args.Error(1)
}

func (m *mockEffectivePlanService) GetUserEffectivePlanForOrg(userID string, orgID *uuid.UUID) (*paymentServices.EffectivePlanResult, error) {
	args := m.Called(userID, orgID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*paymentServices.EffectivePlanResult), args.Error(1)
}

func (m *mockEffectivePlanService) CheckEffectiveUsageLimit(userID string, metricType string, increment int64) (*paymentServices.UsageLimitCheck, error) {
	args := m.Called(userID, metricType, increment)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*paymentServices.UsageLimitCheck), args.Error(1)
}

func (m *mockEffectivePlanService) CheckEffectiveUsageLimitForOrg(userID string, orgID *uuid.UUID, metricType string, increment int64) (*paymentServices.UsageLimitCheck, error) {
	args := m.Called(userID, orgID, metricType, increment)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*paymentServices.UsageLimitCheck), args.Error(1)
}

func (m *mockEffectivePlanService) CheckEffectiveUsageLimitFromResult(result *paymentServices.EffectivePlanResult, userID string, metricType string, increment int64) (*paymentServices.UsageLimitCheck, error) {
	args := m.Called(result, userID, metricType, increment)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*paymentServices.UsageLimitCheck), args.Error(1)
}

// makePlan returns a minimal SubscriptionPlan for testing.
func makePlan(maxConcurrent int) *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		Name:                   "Test Plan",
		MaxConcurrentTerminals: maxConcurrent,
		MaxCourses:             10,
	}
}

// TestCheckLimit_UsesContextPlan_SkipsPlanResolution verifies that when
// InjectEffectivePlan has already stored the plan result in the Gin context,
// CheckLimit reads it from there and does NOT call GetUserEffectivePlanForOrg
// (i.e. CheckEffectiveUsageLimitFromResult is called instead of
// CheckEffectiveUsageLimitForOrg).
func TestCheckLimit_UsesContextPlan_SkipsPlanResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := freshTestDB(t)
	svc := &mockEffectivePlanService{}

	plan := makePlan(3)
	contextResult := &paymentServices.EffectivePlanResult{
		Plan:   plan,
		Source: paymentServices.PlanSourcePersonal,
	}

	allowedCheck := &paymentServices.UsageLimitCheck{
		Allowed:        true,
		CurrentUsage:   1,
		Limit:          3,
		RemainingUsage: 2,
		UserID:         "user-1",
		MetricType:     "concurrent_terminals",
		Source:         paymentServices.PlanSourcePersonal,
	}

	// Expect CheckEffectiveUsageLimitFromResult to be called once with the pre-resolved plan.
	svc.On("CheckEffectiveUsageLimitFromResult", contextResult, "user-1", "concurrent_terminals", int64(1)).
		Return(allowedCheck, nil)

	// CheckEffectiveUsageLimitForOrg must NOT be called.
	// (If it were called, testify would fail with an unexpected call.)

	router := gin.New()
	router.POST("/test",
		func(ctx *gin.Context) {
			// Simulate what InjectEffectivePlan sets in the context.
			ctx.Set("userId", "user-1")
			ctx.Set("effective_plan_result", contextResult)
			ctx.Set("subscription_plan", plan)
			ctx.Next()
		},
		paymentMiddleware.CheckLimit(svc, db, "concurrent_terminals"),
		func(ctx *gin.Context) {
			ctx.Status(http.StatusOK)
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	svc.AssertExpectations(t)
	// Confirm GetUserEffectivePlanForOrg was never called.
	svc.AssertNotCalled(t, "GetUserEffectivePlanForOrg", mock.Anything, mock.Anything)
	svc.AssertNotCalled(t, "CheckEffectiveUsageLimitForOrg", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestCheckLimit_NoContextPlan_FallsBackToFullResolution verifies that when
// no plan is stored in the Gin context (InjectEffectivePlan absent from chain),
// CheckLimit falls back to CheckEffectiveUsageLimitForOrg for full resolution.
func TestCheckLimit_NoContextPlan_FallsBackToFullResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := freshTestDB(t)
	svc := &mockEffectivePlanService{}

	allowedCheck := &paymentServices.UsageLimitCheck{
		Allowed:        true,
		CurrentUsage:   0,
		Limit:          3,
		RemainingUsage: 3,
		UserID:         "user-2",
		MetricType:     "concurrent_terminals",
		Source:         paymentServices.PlanSourcePersonal,
	}

	// No "effective_plan_result" in context → should call CheckEffectiveUsageLimitForOrg.
	svc.On("CheckEffectiveUsageLimitForOrg", "user-2", (*uuid.UUID)(nil), "concurrent_terminals", int64(1)).
		Return(allowedCheck, nil)

	router := gin.New()
	router.POST("/test",
		func(ctx *gin.Context) {
			ctx.Set("userId", "user-2")
			// Deliberately do NOT set "effective_plan_result" in context.
			ctx.Next()
		},
		paymentMiddleware.CheckLimit(svc, db, "concurrent_terminals"),
		func(ctx *gin.Context) {
			ctx.Status(http.StatusOK)
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	svc.AssertExpectations(t)
	svc.AssertNotCalled(t, "CheckEffectiveUsageLimitFromResult", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestCheckLimit_ContextPlanAllows_RequestPasses verifies that an allowed limit
// check from the context plan propagates correctly and lets the handler run.
func TestCheckLimit_ContextPlanAllows_RequestPasses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := freshTestDB(t)
	svc := &mockEffectivePlanService{}

	plan := makePlan(5)
	contextResult := &paymentServices.EffectivePlanResult{
		Plan:   plan,
		Source: paymentServices.PlanSourceOrganization,
	}

	allowedCheck := &paymentServices.UsageLimitCheck{
		Allowed:        true,
		CurrentUsage:   2,
		Limit:          5,
		RemainingUsage: 3,
		UserID:         "user-3",
		MetricType:     "concurrent_terminals",
		Source:         paymentServices.PlanSourceOrganization,
	}

	svc.On("CheckEffectiveUsageLimitFromResult", contextResult, "user-3", "concurrent_terminals", int64(1)).
		Return(allowedCheck, nil)

	handlerCalled := false
	router := gin.New()
	router.POST("/test",
		func(ctx *gin.Context) {
			ctx.Set("userId", "user-3")
			ctx.Set("effective_plan_result", contextResult)
			ctx.Next()
		},
		paymentMiddleware.CheckLimit(svc, db, "concurrent_terminals"),
		func(ctx *gin.Context) {
			handlerCalled = true
			ctx.Status(http.StatusCreated)
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.True(t, handlerCalled, "downstream handler should run when limit allows")
	svc.AssertExpectations(t)
}

// TestCheckLimit_ContextPlanBlocks_Returns403 verifies that when the context plan
// indicates the limit is exceeded, CheckLimit aborts with 403.
func TestCheckLimit_ContextPlanBlocks_Returns403(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := freshTestDB(t)
	svc := &mockEffectivePlanService{}

	plan := makePlan(1)
	contextResult := &paymentServices.EffectivePlanResult{
		Plan:   plan,
		Source: paymentServices.PlanSourcePersonal,
	}

	blockedCheck := &paymentServices.UsageLimitCheck{
		Allowed:        false,
		CurrentUsage:   1,
		Limit:          1,
		RemainingUsage: 0,
		Message:        "Usage limit exceeded for concurrent_terminals. Current: 1, Limit: 1",
		UserID:         "user-4",
		MetricType:     "concurrent_terminals",
		Source:         paymentServices.PlanSourcePersonal,
	}

	svc.On("CheckEffectiveUsageLimitFromResult", contextResult, "user-4", "concurrent_terminals", int64(1)).
		Return(blockedCheck, nil)

	handlerCalled := false
	router := gin.New()
	router.POST("/test",
		func(ctx *gin.Context) {
			ctx.Set("userId", "user-4")
			ctx.Set("effective_plan_result", contextResult)
			ctx.Next()
		},
		paymentMiddleware.CheckLimit(svc, db, "concurrent_terminals"),
		func(ctx *gin.Context) {
			handlerCalled = true
			ctx.Status(http.StatusCreated)
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.False(t, handlerCalled, "downstream handler must not run when limit is exceeded")
	svc.AssertExpectations(t)
}
