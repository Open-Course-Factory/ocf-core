// tests/payment/adminRouteProtection_test.go
//
// Verifies that admin-only payment endpoints reject non-admin users with 403.
//
// Protection is enforced at two levels:
//   1. Casbin RBAC middleware (Layer 1): routes registered with "administrator" policy
//      reject non-admin users before they reach the controller.
//   2. Controller-level isAdmin() checks (defense-in-depth): some handlers still
//      check admin status inline as an extra safeguard.
//
// Endpoints protected by Casbin middleware only (no inline check):
//   - POST /user-subscriptions/sync-existing
//   - POST /user-subscriptions/users/:user_id/sync
//   - POST /user-subscriptions/sync-missing-metadata
//   - POST /user-subscriptions/link/:subscription_id
//
// Endpoints with both Casbin middleware AND inline controller check:
//   - POST /user-subscriptions/admin-assign
//   - GET  /user-subscriptions/analytics
package payment_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	paymentController "soli/formations/src/payment/routes"
	testTools "soli/formations/tests/testTools"
)

// setupAdminRouteTestRouter mounts the real UserSubscriptionRoutes behind the
// real AuthManagement (see installRealGateway) on an empty database.
func setupAdminRouteTestRouter(t *testing.T) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	db := freshTestDB(t)
	installRealGateway(t)

	router := gin.New()
	paymentController.UserSubscriptionRoutes(router.Group("/api/v1"), db)

	return router
}

// setupDirectControllerRouter creates a router that registers the actual controller
// methods WITHOUT AuthManagement(). Instead, a mock middleware sets userId and
// userRoles directly, simulating a user who has passed authentication.
//
// This approach tests whether the CONTROLLER methods themselves properly check
// admin status. If there is no admin check in the controller, the endpoint will
// proceed to business logic (and likely fail with 500 due to no real DB/Stripe).
func setupDirectControllerRouter(t *testing.T, role string) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	db := freshTestDB(t)

	router := gin.New()

	// Mock auth middleware that simulates an authenticated user
	mockAuth := func(c *gin.Context) {
		c.Set("userId", "test-non-admin-user")
		c.Set("userRoles", []string{role})
		c.Next()
	}

	controller := paymentController.NewSubscriptionController(db)

	routes := router.Group("/api/v1/user-subscriptions")

	// Admin-only endpoints with inline controller-level admin checks
	// (defense-in-depth, in addition to Casbin middleware protection)
	routes.POST("/admin-assign", mockAuth, controller.AdminAssignSubscription)
	routes.GET("/analytics", mockAuth, controller.GetSubscriptionAnalytics)

	return router
}

// TestAdminRouteProtection_NonAdminRejected verifies that endpoints with inline
// controller-level admin checks return 403 Forbidden when accessed by a non-admin
// (member) user.
//
// Note: endpoints protected only by Casbin middleware (sync-existing, users/:user_id/sync,
// sync-missing-metadata, link/:subscription_id) are NOT tested here because their
// protection is at the middleware layer, not the controller layer.
func TestAdminRouteProtection_NonAdminRejected(t *testing.T) {
	router := setupDirectControllerRouter(t, "member")

	adminEndpoints := []struct {
		method      string
		path        string
		body        string
		description string
	}{
		{
			method:      "POST",
			path:        "/api/v1/user-subscriptions/admin-assign",
			body:        `{"user_id":"some-user","plan_id":"` + uuid.New().String() + `","duration_days":30}`,
			description: "Admin subscription assignment",
		},
		{
			method:      "GET",
			path:        "/api/v1/user-subscriptions/analytics",
			body:        "",
			description: "Subscription analytics",
		},
	}

	for _, ep := range adminEndpoints {
		t.Run(ep.description+"_rejects_member", func(t *testing.T) {
			var req *http.Request
			if ep.body != "" {
				req = httptest.NewRequest(ep.method, ep.path, bytes.NewBufferString(ep.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(ep.method, ep.path, nil)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Admin-only endpoints MUST return 403 for non-admin users.
			// Any other status (200, 400, 500) means the admin check is missing
			// or broken at both the middleware and controller level.
			assert.Equal(t, http.StatusForbidden, w.Code,
				"Endpoint %s %s (%s) should return 403 Forbidden for non-admin users, got %d. "+
					"Response body: %s",
				ep.method, ep.path, ep.description, w.Code, w.Body.String())

			// Verify the response contains an appropriate error message
			if w.Code == http.StatusForbidden {
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err, "Response should be valid JSON")

				// Check that error message indicates admin access required
				hasAdminMsg := false
				for _, v := range response {
					if str, ok := v.(string); ok {
						if str == "Access denied - admin role required" ||
							str == "Admin access required" ||
							str == "Access denied" {
							hasAdminMsg = true
							break
						}
					}
				}
				assert.True(t, hasAdminMsg,
					"403 response for %s should contain admin access denied message, got: %v",
					ep.path, response)
			}
		})
	}
}

// TestAdminRouteProtection_AdminAllowed verifies that endpoints with inline
// controller-level admin checks let an administrator through to a successful
// answer.
func TestAdminRouteProtection_AdminAllowed(t *testing.T) {
	router := setupDirectControllerRouter(t, "administrator")
	plan := createTestPlan(t, "Plan to assign", 0)
	// The assignment looks the target user up in Casdoor.
	testTools.StartFakeCasdoorUsers(t, 0).AddUser("some-user", "Some User", "some-user", "some-user@example.test")

	adminEndpoints := []struct {
		method      string
		path        string
		body        string
		description string
	}{
		{
			method:      "POST",
			path:        "/api/v1/user-subscriptions/admin-assign",
			body:        `{"user_id":"some-user","plan_id":"` + plan.ID.String() + `","duration_days":30}`,
			description: "Admin subscription assignment",
		},
		{
			method:      "GET",
			path:        "/api/v1/user-subscriptions/analytics",
			body:        "",
			description: "Subscription analytics",
		},
	}

	for _, ep := range adminEndpoints {
		t.Run(ep.description+"_allows_admin", func(t *testing.T) {
			var req *http.Request
			if ep.body != "" {
				req = httptest.NewRequest(ep.method, ep.path, bytes.NewBufferString(ep.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(ep.method, ep.path, nil)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code,
				"Endpoint %s %s (%s) should succeed for admin users. Response: %s",
				ep.method, ep.path, ep.description, w.Body.String())
		})
	}
}

// TestAdminRouteProtection_RealRoutesRejectMember sends each admin-only
// endpoint through the real route registration: no token is refused with 401,
// and a signed-in member is refused with 403 by the gateway, before the email
// verification middleware or the handler runs.
func TestAdminRouteProtection_RealRoutesRejectMember(t *testing.T) {
	router := setupAdminRouteTestRouter(t)

	adminEndpoints := []struct {
		method      string
		path        string
		description string
	}{
		{"POST", "/api/v1/user-subscriptions/admin-assign", "Admin assign"},
		{"POST", "/api/v1/user-subscriptions/sync-existing", "Sync existing"},
		{"GET", "/api/v1/user-subscriptions/analytics", "Analytics"},
		{"POST", "/api/v1/user-subscriptions/users/user123/sync", "User sync"},
		{"POST", "/api/v1/user-subscriptions/sync-missing-metadata", "Sync missing metadata"},
		{"POST", "/api/v1/user-subscriptions/link/sub_123", "Link subscription"},
	}

	for _, ep := range adminEndpoints {
		t.Run(ep.description, func(t *testing.T) {
			anonymous := requestAs(router, "", ep.method, ep.path, "")
			assert.Equal(t, http.StatusUnauthorized, anonymous.Code, anonymous.Body.String())

			assertRefusedByGateway(t, requestAs(router, gatewayMember, ep.method, ep.path, ""))
		})
	}
}
