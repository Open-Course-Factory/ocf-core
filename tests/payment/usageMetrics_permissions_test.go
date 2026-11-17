// tests/payment/usageMetrics_permissions_test.go
//
// Verifies that usage metrics mutation endpoints (increment and reset) are
// restricted to administrators only. Members must not be able to call these
// endpoints, as doing so would allow them to bypass subscription usage limits.
//
// The GET /usage-metrics/user endpoint remains member-accessible (self-scoped).
package payment_tests

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	paymentController "soli/formations/src/payment/routes"
)

const incrementCoursesBody = `{"metric_type":"courses_created","increment":1}`

// setupUsageMetricsRealRouter mounts the real UsageMetricsRoutes behind the
// real AuthManagement (see installRealGateway) on an empty database.
func setupUsageMetricsRealRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	db := freshTestDB(t)
	installRealGateway(t)

	router := gin.New()
	paymentController.UsageMetricsRoutes(router.Group("/api/v1"), db)

	return router, db
}

// TestUsageMetricsReset_MemberCannotCallReset verifies that a member is
// refused by the gateway on the reset endpoint.
func TestUsageMetricsReset_MemberCannotCallReset(t *testing.T) {
	router, _ := setupUsageMetricsRealRouter(t)

	w := requestAs(router, gatewayMember, "POST", "/api/v1/usage-metrics/reset", "")

	assertRefusedByGateway(t, w)
}

// TestUsageMetricsIncrement_MemberCannotCallIncrement verifies that a member
// is refused by the gateway on the increment endpoint.
func TestUsageMetricsIncrement_MemberCannotCallIncrement(t *testing.T) {
	router, db := setupUsageMetricsRealRouter(t)
	// With a subscription the handler would answer 200 if the member reached it.
	createUserSubscription(t, db, gatewayMember, seedAnyPlan(t, db))

	w := requestAs(router, gatewayMember, "POST", "/api/v1/usage-metrics/increment", incrementCoursesBody)

	assertRefusedByGateway(t, w)
}

// TestUsageMetricsReset_AdminCanCallReset verifies that an administrator
// can reset their own usage.
func TestUsageMetricsReset_AdminCanCallReset(t *testing.T) {
	router, _ := setupUsageMetricsRealRouter(t)

	w := requestAs(router, gatewayAdmin, "POST", "/api/v1/usage-metrics/reset", "")

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// TestUsageMetricsIncrement_AdminCanCallIncrement verifies that an administrator
// can increment their own usage.
func TestUsageMetricsIncrement_AdminCanCallIncrement(t *testing.T) {
	router, db := setupUsageMetricsRealRouter(t)
	createUserSubscription(t, db, gatewayAdmin, seedAnyPlan(t, db))

	w := requestAs(router, gatewayAdmin, "POST", "/api/v1/usage-metrics/increment", incrementCoursesBody)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// TestUsageMetricsGet_MemberCanReadOwnMetrics verifies that the GET endpoint
// remains accessible to regular members for their own metrics.
func TestUsageMetricsGet_MemberCanReadOwnMetrics(t *testing.T) {
	router, _ := setupUsageMetricsRealRouter(t)

	w := requestAs(router, gatewayMember, "GET", "/api/v1/usage-metrics/user", "")

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// TestUsageMetricsReset_AdminCanTargetSpecificUser verifies that an admin
// can reset metrics for a specific user via the user_id query parameter.
func TestUsageMetricsReset_AdminCanTargetSpecificUser(t *testing.T) {
	router, _ := setupUsageMetricsRealRouter(t)

	w := requestAs(router, gatewayAdmin, "POST", "/api/v1/usage-metrics/reset?user_id=target-user", "")

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// TestUsageMetricsIncrement_AdminCanTargetSpecificUser verifies that an admin
// can increment metrics for a specific user via the user_id query parameter.
func TestUsageMetricsIncrement_AdminCanTargetSpecificUser(t *testing.T) {
	router, db := setupUsageMetricsRealRouter(t)
	createUserSubscription(t, db, "target-user", seedAnyPlan(t, db))

	w := requestAs(router, gatewayAdmin, "POST", "/api/v1/usage-metrics/increment?user_id=target-user", incrementCoursesBody)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
