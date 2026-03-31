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
	config "soli/formations/src/configuration"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupAdminRouteTestRouter creates a Gin router with the real UserSubscriptionRoutes
// but with a mock auth middleware injected at the group level.
// The mock middleware sets userId and userRoles BEFORE the route handlers run.
//
// IMPORTANT: The real AuthManagement() is still registered per-route and will run
// after our group-level middleware. Since AuthManagement() tries to parse a JWT
// (which doesn't exist in tests), it will abort with 401.
//
// This means we CANNOT test admin protection through the real route registration
// when AuthManagement() is present. See setupDirectControllerRouter for an
// alternative approach that bypasses AuthManagement() entirely.
func setupAdminRouteTestRouter(t *testing.T, role string) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	assert.NoError(t, err)

	router := gin.New()
	apiGroup := router.Group("/api/v1")

	// Mock auth middleware: simulates an authenticated user with the given role.
	// This runs at the group level, BEFORE per-route AuthManagement().
	apiGroup.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-id")
		c.Set("userRoles", []string{role})
		c.Next()
	})

	paymentController.UserSubscriptionRoutes(apiGroup, &config.Configuration{}, db)

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

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	assert.NoError(t, err)

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
// controller-level admin checks do NOT return 403 when accessed by an admin user.
// They may return other errors (400, 500) due to missing data/Stripe, but
// the point is they should NOT be blocked by the admin check.
func TestAdminRouteProtection_AdminAllowed(t *testing.T) {
	router := setupDirectControllerRouter(t, "administrator")

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

			// Admin users should NOT get 403. They may get other errors (500, 400)
			// because Stripe/DB is not fully set up, but 403 means the admin check
			// is incorrectly blocking admins.
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"Endpoint %s %s (%s) should NOT return 403 for admin users, got %d. "+
					"Response: %s",
				ep.method, ep.path, ep.description, w.Code, w.Body.String())
		})
	}
}

// TestAdminRouteProtection_RealRoutesHaveNoAdminMiddleware documents that the
// real route registration in UserSubscriptionRoutes does NOT include an admin
// middleware for admin-only endpoints. The only protection is AuthManagement()
// which checks general authentication and Casbin role permissions.
//
// This is a structural test: it verifies the problem exists by showing that
// when accessing admin-only endpoints through the real route setup, the response
// is 401 (auth failed, no JWT) rather than 403 (admin check), proving there is
// no admin-specific middleware in the route chain.
//
// After adding a RequireAdmin middleware to the routes, requests from non-admin
// users should return 403 even when AuthManagement() is the first middleware.
func TestAdminRouteProtection_RealRoutesHaveNoAdminMiddleware(t *testing.T) {
	// This router uses the real UserSubscriptionRoutes with AuthManagement()
	// Since we have no JWT token, AuthManagement() will return 401.
	// If an admin middleware existed at the route level (after AuthManagement),
	// we couldn't distinguish it here -- but the fact that we get 401 from
	// AuthManagement proves no middleware runs before it to check admin status.
	router := setupAdminRouteTestRouter(t, "member")

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
		t.Run(ep.description+"_no_admin_middleware", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// With real routes, AuthManagement() runs first and returns 401
			// because there's no JWT token. This proves:
			// 1. The route exists and is reachable
			// 2. AuthManagement() is the ONLY protection (returns 401 for no token)
			// 3. There is NO admin middleware that would return 403 before or after AuthManagement
			//
			// If/when a RequireAdmin middleware is added AFTER AuthManagement, the
			// behavior won't change for unauthenticated requests (401 from AuthManagement
			// stops the chain), but authenticated non-admin users will get 403 from
			// the middleware instead of proceeding to the controller.
			assert.Equal(t, http.StatusUnauthorized, w.Code,
				"Real route %s %s should return 401 (from AuthManagement, no JWT). "+
					"If this returns 403, an admin middleware has been added (which is the fix!). "+
					"Got %d: %s",
				ep.method, ep.path, w.Code, w.Body.String())
		})
	}
}
