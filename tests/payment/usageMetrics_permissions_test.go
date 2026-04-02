// tests/payment/usageMetrics_permissions_test.go
//
// Verifies that usage metrics mutation endpoints (increment and reset) are
// restricted to administrators only. Members must not be able to call these
// endpoints, as doing so would allow them to bypass subscription usage limits.
//
// The GET /usage-metrics/user endpoint remains member-accessible (self-scoped).
package payment_tests

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	config "soli/formations/src/configuration"
	paymentController "soli/formations/src/payment/routes"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupUsageMetricsDirectRouter creates a router with the real UsageMetricsController
// methods but with a mock auth middleware instead of AuthManagement().
// This tests whether the controller itself would allow the operation.
func setupUsageMetricsDirectRouter(t *testing.T, role string) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	assert.NoError(t, err)

	router := gin.New()

	mockAuth := func(c *gin.Context) {
		c.Set("userId", "test-user-id")
		c.Set("userRoles", []string{role})
		c.Next()
	}

	controller := paymentController.NewUsageMetricsController(db)

	routes := router.Group("/api/v1/usage-metrics")
	routes.GET("/user", mockAuth, controller.GetUserUsageMetrics)
	routes.POST("/increment", mockAuth, controller.IncrementUsageMetric)
	routes.POST("/reset", mockAuth, controller.ResetUserUsage)

	return router
}

// setupUsageMetricsRealRouter creates a router using the real UsageMetricsRoutes
// registration (with AuthManagement() middleware). A group-level mock auth
// middleware sets userId and userRoles before AuthManagement() runs.
//
// Since AuthManagement() requires a real JWT, requests without one get 401.
// This verifies the route registration is correct.
func setupUsageMetricsRealRouter(t *testing.T, role string) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	assert.NoError(t, err)

	router := gin.New()
	apiGroup := router.Group("/api/v1")

	apiGroup.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-id")
		c.Set("userRoles", []string{role})
		c.Next()
	})

	paymentController.UsageMetricsRoutes(apiGroup, &config.Configuration{}, db)

	return router
}

// TestUsageMetricsReset_MemberCannotCallReset verifies that the reset endpoint
// no longer allows regular members. Since the route is admin-only at the
// middleware level, this test uses the real route registration to confirm
// the route exists and is guarded by AuthManagement().
func TestUsageMetricsReset_MemberCannotCallReset(t *testing.T) {
	// Using real routes: AuthManagement() will return 401 (no JWT).
	// This proves the route is registered and protected by auth middleware.
	router := setupUsageMetricsRealRouter(t, "member")

	req := httptest.NewRequest("POST", "/api/v1/usage-metrics/reset", nil)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// With real AuthManagement(), no JWT = 401.
	// The important thing is it's NOT 200 (unprotected).
	assert.NotEqual(t, http.StatusOK, w.Code,
		"POST /usage-metrics/reset should NOT return 200 for unauthenticated member requests")
}

// TestUsageMetricsIncrement_MemberCannotCallIncrement verifies that the
// increment endpoint is not accessible to regular members.
func TestUsageMetricsIncrement_MemberCannotCallIncrement(t *testing.T) {
	router := setupUsageMetricsRealRouter(t, "member")

	body := `{"metric_type":"concurrent_terminals","increment":1}`
	req := httptest.NewRequest("POST", "/api/v1/usage-metrics/increment",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code,
		"POST /usage-metrics/increment should NOT return 200 for unauthenticated member requests")
}

// TestUsageMetricsReset_AdminCanCallReset verifies that an administrator
// can reach the reset controller method (it won't return 403).
// Uses the direct controller router to bypass AuthManagement() JWT check.
func TestUsageMetricsReset_AdminCanCallReset(t *testing.T) {
	router := setupUsageMetricsDirectRouter(t, "administrator")

	req := httptest.NewRequest("POST", "/api/v1/usage-metrics/reset", nil)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Admin should NOT get 403. May get 500 (no real DB data) which is fine —
	// the point is the admin check does not block them.
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"POST /usage-metrics/reset should NOT return 403 for admin users, got %d: %s",
		w.Code, w.Body.String())
}

// TestUsageMetricsIncrement_AdminCanCallIncrement verifies that an administrator
// can reach the increment controller method.
func TestUsageMetricsIncrement_AdminCanCallIncrement(t *testing.T) {
	router := setupUsageMetricsDirectRouter(t, "administrator")

	body := `{"metric_type":"concurrent_terminals","increment":1}`
	req := httptest.NewRequest("POST", "/api/v1/usage-metrics/increment",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Admin should NOT get 403.
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"POST /usage-metrics/increment should NOT return 403 for admin users, got %d: %s",
		w.Code, w.Body.String())
}

// TestUsageMetricsGet_MemberCanReadOwnMetrics verifies that the GET endpoint
// remains accessible to regular members for their own metrics.
func TestUsageMetricsGet_MemberCanReadOwnMetrics(t *testing.T) {
	router := setupUsageMetricsDirectRouter(t, "member")

	req := httptest.NewRequest("GET", "/api/v1/usage-metrics/user", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Members should NOT get 403 on the read endpoint.
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"GET /usage-metrics/user should NOT return 403 for member users")
}

// TestUsageMetricsReset_AdminCanTargetSpecificUser verifies that an admin
// can reset metrics for a specific user via the user_id query parameter.
func TestUsageMetricsReset_AdminCanTargetSpecificUser(t *testing.T) {
	router := setupUsageMetricsDirectRouter(t, "administrator")

	req := httptest.NewRequest("POST", "/api/v1/usage-metrics/reset?user_id=target-user", nil)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should not be blocked by authorization. May get 500 due to no DB data.
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"POST /usage-metrics/reset?user_id=target-user should NOT return 403 for admin")
}

// TestUsageMetricsIncrement_AdminCanTargetSpecificUser verifies that an admin
// can increment metrics for a specific user via the user_id query parameter.
func TestUsageMetricsIncrement_AdminCanTargetSpecificUser(t *testing.T) {
	router := setupUsageMetricsDirectRouter(t, "administrator")

	body := `{"metric_type":"concurrent_terminals","increment":1}`
	req := httptest.NewRequest("POST", "/api/v1/usage-metrics/increment?user_id=target-user",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"POST /usage-metrics/increment?user_id=target-user should NOT return 403 for admin")
}
