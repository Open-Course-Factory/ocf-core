package authorization_tests

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
)

// ---------------------------------------------------------------------------
// Regression test for GitLab issue #258
// ---------------------------------------------------------------------------
//
// Bug: Layer2Enforcement() middleware is registered at the apiGroup level
// (main.go:180), while AuthManagement() — which populates `userId` / `userRoles`
// into the Gin context — is registered per-route or on sub-groups
// (e.g. src/payment/routes/bulkLicenseRoutes.go:26).
//
// Gin executes middleware in registration order. So for routes that rely on a
// per-route/sub-group AuthManagement(), the chain at request time is:
//
//     Layer2Enforcement()  ← apiGroup-level, runs FIRST
//     AuthManagement()     ← sub-group level, runs SECOND
//     handler()            ← runs THIRD
//
// When Layer2Enforcement() runs, ctx.Get("userId") returns !exists, so per
// `enforcement_middleware.go:165-170`, the middleware short-circuits by calling
// ctx.Next() and returns BEFORE invoking the registered access enforcer.
//
// The net effect: the declared RouteRegistry rule (EntityOwner, OrgRole,
// AdminOnly, GroupRole, ...) is NEVER enforced at request time for routes that
// use per-route AuthManagement. Only AuthManagement's own RBAC check runs,
// which is Layer 1 — Layer 2 is silently bypassed.
//
// This test documents the bug. It is EXPECTED TO FAIL on main today. It will
// pass once the middleware ordering is fixed (e.g. by registering
// Layer2Enforcement AFTER AuthManagement on the relevant groups, or by making
// AuthManagement run before Layer2Enforcement globally).

// spyEnforcer records whether it was invoked and denies every request.
type spyEnforcer struct {
	called atomic.Int32
}

func (s *spyEnforcer) wasCalled() bool {
	return s.called.Load() > 0
}

func (s *spyEnforcer) handler() access.AccessEnforcer {
	return func(ctx *gin.Context, rule access.AccessRule, userID string, roles []string) bool {
		s.called.Add(1)
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":  "Access denied",
			"detail": "spy enforcer always denies",
		})
		return false
	}
}

// fakeAuthManagement mimics the real AuthManagement middleware's side effect:
// it populates ctx with a userId + roles (as if a JWT had been validated).
// It does nothing else — the real middleware also does Casbin RBAC, but for
// this test we only care about the userId injection because that is what
// Layer2Enforcement keys off.
func fakeAuthManagement(userID string, roles []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	}
}

// TestLayer2Enforcement_PerRouteAuthManagement_BypassesEnforcement reproduces
// the production middleware ordering that causes Layer 2 to be bypassed.
//
// It also contains a sanity sub-test showing that when the ordering is
// correct (AuthManagement BEFORE Layer2Enforcement), the spy IS called and
// the request IS rejected — proving the spy itself works.
func TestLayer2Enforcement_PerRouteAuthManagement_BypassesEnforcement(t *testing.T) {
	// Route contract used by both sub-tests. The spy enforcer is registered
	// against this access rule type and must be invoked for every request
	// that reaches a route with this declaration.
	const (
		routePath   = "/api/v1/test/owned-resource/:id"
		routeMethod = "POST"
		userID      = "user-A"
	)
	roles := []string{"member"}

	// -------------------------------------------------------------------
	// Sub-test 1: production ordering — Layer2Enforcement BEFORE
	// AuthManagement (the bug).
	//
	// Expectation (the contract that SHOULD hold): the spy enforcer IS
	// invoked and the request IS rejected with 403.
	//
	// Reality today: the spy is NEVER invoked and the request returns 200,
	// because Layer2Enforcement short-circuits on missing userId.
	// -------------------------------------------------------------------
	t.Run("production_ordering_bypasses_layer2", func(t *testing.T) {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
		defer func() {
			access.RouteRegistry.Reset()
			access.ResetEnforcers()
		}()

		// Register the route with a restrictive access rule. The spy enforcer
		// will always deny — so if Layer 2 enforcement actually ran, the
		// request would be rejected with 403.
		access.RouteRegistry.Register("test", access.RoutePermission{
			Path:   routePath,
			Method: routeMethod,
			Role:   "member",
			Access: access.AccessRule{
				Type:   access.EntityOwner,
				Entity: "TestEntity",
				Field:  "UserID",
			},
		})

		spy := &spyEnforcer{}
		access.RegisterAccessEnforcer(access.EntityOwner, spy.handler())

		gin.SetMode(gin.TestMode)
		r := gin.New()

		// Mirror main.go:180 — Layer2Enforcement is attached to the apiGroup.
		apiGroup := r.Group("/api/v1")
		apiGroup.Use(access.Layer2Enforcement())

		// Mirror bulkLicenseRoutes.go:26 — AuthManagement is attached to a
		// sub-group AFTER Layer2Enforcement, so the chain for this route is:
		// Layer2Enforcement → fakeAuthManagement → handler.
		subGroup := apiGroup.Group("/test")
		subGroup.Use(fakeAuthManagement(userID, roles))
		subGroup.POST("/owned-resource/:id", okHandler)

		req := httptest.NewRequest(routeMethod, "/api/v1/test/owned-resource/abc-123", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		// These assertions describe what SHOULD happen once the ordering bug
		// is fixed. They fail on main today because Layer 2 is bypassed.
		assert.True(t, spy.wasCalled(),
			"Layer 2 enforcer MUST be invoked for routes using per-route AuthManagement "+
				"(bug #258: middleware ordering causes Layer 2 to short-circuit before "+
				"AuthManagement populates userId)")
		assert.Equal(t, http.StatusForbidden, w.Code,
			"restrictive Layer 2 rule MUST reject the request with 403 "+
				"(bug #258: declared RouteRegistry rule is never enforced at request time)")
	})

	// -------------------------------------------------------------------
	// Sub-test 2: correct ordering — AuthManagement BEFORE
	// Layer2Enforcement.
	//
	// This demonstrates that the spy enforcer works correctly when given a
	// userId. If this sub-test fails, the bug is somewhere else than the
	// middleware ordering.
	// -------------------------------------------------------------------
	t.Run("reversed_ordering_enforces_layer2", func(t *testing.T) {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
		defer func() {
			access.RouteRegistry.Reset()
			access.ResetEnforcers()
		}()

		access.RouteRegistry.Register("test", access.RoutePermission{
			Path:   routePath,
			Method: routeMethod,
			Role:   "member",
			Access: access.AccessRule{
				Type:   access.EntityOwner,
				Entity: "TestEntity",
				Field:  "UserID",
			},
		})

		spy := &spyEnforcer{}
		access.RegisterAccessEnforcer(access.EntityOwner, spy.handler())

		gin.SetMode(gin.TestMode)
		r := gin.New()

		// Reversed ordering: AuthManagement (sets userId) runs FIRST, then
		// Layer2Enforcement runs with a populated context.
		apiGroup := r.Group("/api/v1")
		apiGroup.Use(fakeAuthManagement(userID, roles))
		apiGroup.Use(access.Layer2Enforcement())

		subGroup := apiGroup.Group("/test")
		subGroup.POST("/owned-resource/:id", okHandler)

		req := httptest.NewRequest(routeMethod, "/api/v1/test/owned-resource/abc-123", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.True(t, spy.wasCalled(),
			"sanity: with AuthManagement before Layer2Enforcement, the spy must be invoked")
		assert.Equal(t, http.StatusForbidden, w.Code,
			"sanity: with correct ordering, restrictive Layer 2 rule must return 403")
	})
}
