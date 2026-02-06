package payment_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	paymentController "soli/formations/src/payment/routes"
	config "soli/formations/src/configuration"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupPaymentTestRouter creates a gin router with payment routes and a mock auth middleware
// that sets userId but does NOT make Casdoor available (simulating an unverified user
// whose email_verified status cannot be checked by the verification middleware).
func setupPaymentTestRouter(t *testing.T) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	router := gin.New()
	apiGroup := router.Group("/api/v1")

	// Mock auth middleware that just sets userId (simulating authenticated user)
	apiGroup.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-unverified")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})

	paymentController.UserSubscriptionRoutes(apiGroup, &config.Configuration{}, db)

	return router
}

// TestUnverifiedUser_CanReadSubscriptionInfo verifies that read-only subscription
// endpoints do NOT require email verification.
func TestUnverifiedUser_CanReadSubscriptionInfo(t *testing.T) {
	router := setupPaymentTestRouter(t)

	readEndpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/user-subscriptions/current"},
		{"GET", "/api/v1/user-subscriptions/all"},
		{"GET", "/api/v1/user-subscriptions/usage"},
		{"GET", "/api/v1/user-subscriptions/analytics"},
	}

	for _, ep := range readEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Should NOT get 403 EMAIL_NOT_VERIFIED or 401 from verification middleware.
			// May get 404 (no subscription) or 500 (DB not migrated) - that's fine,
			// the point is the email verification middleware did not block the request.
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"Read endpoint %s should not be blocked by email verification", ep.path)
			assert.NotContains(t, w.Body.String(), "EMAIL_NOT_VERIFIED",
				"Read endpoint %s should not return EMAIL_NOT_VERIFIED", ep.path)
		})
	}
}

// TestUnverifiedUser_BlockedFromPaymentActions verifies that payment action
// endpoints DO require email verification.
func TestUnverifiedUser_BlockedFromPaymentActions(t *testing.T) {
	router := setupPaymentTestRouter(t)

	paymentEndpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/user-subscriptions/checkout"},
		{"POST", "/api/v1/user-subscriptions/portal"},
		{"POST", "/api/v1/user-subscriptions/upgrade"},
	}

	for _, ep := range paymentEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// The email verification middleware will try to call Casdoor (which is unavailable
			// in tests), so it returns 401 "User not found". This proves the middleware IS active.
			// In production with a real unverified user, it would return 403 EMAIL_NOT_VERIFIED.
			assert.True(t, w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden,
				"Payment endpoint %s should be blocked (got %d)", ep.path, w.Code)
		})
	}
}
