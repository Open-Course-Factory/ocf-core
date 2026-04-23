package authorization_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
)

// ---------------------------------------------------------------------------
// Regression + hardening tests for GitLab issue #258
// ---------------------------------------------------------------------------
//
// Bug: Layer2Enforcement() middleware is registered at the apiGroup level
// (main.go:180), while AuthManagement() — which historically populated
// `userId` / `userRoles` into the Gin context — is registered per-route or
// on sub-groups (e.g. src/payment/routes/bulkLicenseRoutes.go:26).
//
// Gin executes middleware in registration order. So for routes that rely on a
// per-route/sub-group AuthManagement(), the chain at request time is:
//
//     Layer2Enforcement()  ← apiGroup-level, runs FIRST
//     AuthManagement()     ← sub-group level, runs SECOND
//     handler()            ← runs THIRD
//
// Before the fix, Layer2Enforcement() short-circuited when
// ctx.Get("userId") returned !exists, so the declared RouteRegistry rule
// (EntityOwner, OrgRole, AdminOnly, GroupRole, ...) was NEVER enforced at
// request time for routes using per-route AuthManagement.
//
// Fix (Option C): Layer2Enforcement resolves userId itself from the JWT
// when the context has not yet been populated, so the declared rule is
// enforced regardless of middleware ordering.

// spyEnforcer records whether it was invoked and denies every request.
type spyEnforcer struct {
	called atomic.Int32
}

func (s *spyEnforcer) wasCalled() bool {
	return s.called.Load() > 0
}

func (s *spyEnforcer) callCount() int32 {
	return s.called.Load()
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

// stubResolverCall captures calls to a stubbed user resolver.
type stubResolverCall struct {
	count atomic.Int32
}

func (s *stubResolverCall) increment() {
	s.count.Add(1)
}

func (s *stubResolverCall) callCount() int32 {
	return s.count.Load()
}

// TestLayer2Enforcement_PerRouteAuthManagement_EnforcesViaJWT verifies that
// Option C (Layer2Enforcement resolves userId from the JWT itself when the
// context is empty) makes Layer 2 enforcement robust against middleware
// ordering.
//
// The two sub-tests exercise both orderings of AuthManagement /
// Layer2Enforcement. In BOTH cases the declared RouteRegistry rule MUST be
// evaluated at request time.
func TestLayer2Enforcement_PerRouteAuthManagement_EnforcesViaJWT(t *testing.T) {
	const (
		routePath   = "/api/v1/test/owned-resource/:id"
		routeMethod = "POST"
		userID      = "user-A"
	)
	roles := []string{"member"}

	// -------------------------------------------------------------------
	// Sub-test 1: production ordering — Layer2Enforcement runs BEFORE
	// AuthManagement. Before the fix this silently bypassed Layer 2.
	// With Option C the enforcer parses the JWT itself and the rule runs.
	// -------------------------------------------------------------------
	t.Run("production_ordering_enforces_layer2", func(t *testing.T) {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
		defer func() {
			access.RouteRegistry.Reset()
			access.ResetEnforcers()
		}()

		// Stub the JWT resolver so the test does not depend on a live
		// Casdoor configuration. The resolver returns the same userId /
		// roles that fakeAuthManagement would inject.
		restoreResolver := access.SetUserResolver(func(ctx *gin.Context) (string, []string, error) {
			if ctx.Request.Header.Get("Authorization") == "" {
				return "", nil, fmt.Errorf("missing Authorization header")
			}
			return userID, roles, nil
		})
		defer restoreResolver()

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
		// sub-group AFTER Layer2Enforcement.
		subGroup := apiGroup.Group("/test")
		subGroup.Use(fakeAuthManagement(userID, roles))
		subGroup.POST("/owned-resource/:id", okHandler)

		req := httptest.NewRequest(routeMethod, "/api/v1/test/owned-resource/abc-123", nil)
		req.Header.Set("Authorization", "Bearer fake-jwt")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.True(t, spy.wasCalled(),
			"Option C: Layer 2 enforcer MUST be invoked even when AuthManagement runs after Layer2Enforcement")
		assert.Equal(t, http.StatusForbidden, w.Code,
			"Option C: declared RouteRegistry rule MUST be enforced at request time")
	})

	// -------------------------------------------------------------------
	// Sub-test 2: correct ordering — AuthManagement BEFORE
	// Layer2Enforcement. This demonstrates backward compatibility: when
	// the context already has userId, Layer2Enforcement uses it as-is.
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

// TestLayer2Enforcement_ResolvesUserFromJWT_WhenContextEmpty asserts that
// Layer2Enforcement resolves the userId directly from the JWT when the
// context has not yet been populated (e.g. AuthManagement runs later in
// the chain) and that the resolved values are passed to the access
// enforcer.
func TestLayer2Enforcement_ResolvesUserFromJWT_WhenContextEmpty(t *testing.T) {
	const (
		routePath   = "/api/v1/test/owned-resource/:id"
		routeMethod = "POST"
		userID      = "user-from-jwt"
	)
	resolvedRoles := []string{"member"}

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	stubCalls := &stubResolverCall{}
	restore := access.SetUserResolver(func(ctx *gin.Context) (string, []string, error) {
		stubCalls.increment()
		return userID, resolvedRoles, nil
	})
	defer restore()

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

	var (
		gotUserID string
		gotRoles  []string
	)
	recordingEnforcer := func(ctx *gin.Context, rule access.AccessRule, uid string, r []string) bool {
		gotUserID = uid
		gotRoles = r
		return true // allow the request through so we can check the handler ran
	}
	access.RegisterAccessEnforcer(access.EntityOwner, recordingEnforcer)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	apiGroup := r.Group("/api/v1")
	apiGroup.Use(access.Layer2Enforcement())
	subGroup := apiGroup.Group("/test")
	subGroup.POST("/owned-resource/:id", okHandler)

	req := httptest.NewRequest(routeMethod, "/api/v1/test/owned-resource/abc-123", nil)
	req.Header.Set("Authorization", "Bearer valid-jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, int32(1), stubCalls.callCount(),
		"Layer2Enforcement should invoke the user resolver exactly once")
	assert.Equal(t, userID, gotUserID,
		"access enforcer must receive the userID resolved from the JWT")
	assert.Equal(t, resolvedRoles, gotRoles,
		"access enforcer must receive the roles resolved from the JWT")
	assert.Equal(t, http.StatusOK, w.Code,
		"permissive enforcer should allow the request through")
}

// TestLayer2Enforcement_FallsThroughOnMissingJWT documents that Layer 2 is
// not an authentication checkpoint: when no Authorization header is
// present, the middleware must fall through to the next middleware
// (normally AuthManagement, which returns 401). The access enforcer must
// NOT be invoked.
func TestLayer2Enforcement_FallsThroughOnMissingJWT(t *testing.T) {
	const (
		routePath   = "/api/v1/test/owned-resource/:id"
		routeMethod = "POST"
	)

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	stubCalls := &stubResolverCall{}
	restore := access.SetUserResolver(func(ctx *gin.Context) (string, []string, error) {
		stubCalls.increment()
		if ctx.Request.Header.Get("Authorization") == "" {
			return "", nil, fmt.Errorf("missing Authorization header")
		}
		return "", nil, nil
	})
	defer restore()

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
	apiGroup := r.Group("/api/v1")
	apiGroup.Use(access.Layer2Enforcement())
	subGroup := apiGroup.Group("/test")
	subGroup.POST("/owned-resource/:id", okHandler)

	req := httptest.NewRequest(routeMethod, "/api/v1/test/owned-resource/abc-123", nil)
	// No Authorization header.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.False(t, spy.wasCalled(),
		"enforcer MUST NOT run when the JWT is missing — Layer 2 is not an authentication checkpoint")
	assert.Equal(t, http.StatusOK, w.Code,
		"with no downstream auth middleware, the request reaches the handler (pass-through)")
}

// TestLayer2Enforcement_FallsThroughOnInvalidJWT documents that Layer 2
// falls through when the JWT fails to parse, leaving the 401 response to
// the downstream AuthManagement middleware.
func TestLayer2Enforcement_FallsThroughOnInvalidJWT(t *testing.T) {
	const (
		routePath   = "/api/v1/test/owned-resource/:id"
		routeMethod = "POST"
	)

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	restore := access.SetUserResolver(func(ctx *gin.Context) (string, []string, error) {
		return "", nil, fmt.Errorf("invalid signature")
	})
	defer restore()

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
	apiGroup := r.Group("/api/v1")
	apiGroup.Use(access.Layer2Enforcement())
	subGroup := apiGroup.Group("/test")
	subGroup.POST("/owned-resource/:id", okHandler)

	req := httptest.NewRequest(routeMethod, "/api/v1/test/owned-resource/abc-123", nil)
	req.Header.Set("Authorization", "Bearer tampered-jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.False(t, spy.wasCalled(),
		"enforcer MUST NOT run when JWT parsing fails — downstream middleware rejects with 401")
	assert.Equal(t, http.StatusOK, w.Code,
		"without downstream auth, pass-through lets the handler run (represents 'let AuthManagement decide')")
}

// TestLayer2Enforcement_UserIdAlreadyInContext_SkipsJWTParsing asserts
// backward compatibility: when AuthManagement has already set userId on
// the context, Layer2Enforcement uses it directly and does NOT invoke
// the JWT resolver.
func TestLayer2Enforcement_UserIdAlreadyInContext_SkipsJWTParsing(t *testing.T) {
	const (
		routePath   = "/api/v1/test/owned-resource/:id"
		routeMethod = "POST"
		userID      = "user-preset-in-ctx"
	)
	presetRoles := []string{"member"}

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	stubCalls := &stubResolverCall{}
	restore := access.SetUserResolver(func(ctx *gin.Context) (string, []string, error) {
		stubCalls.increment()
		return "should-not-be-used", nil, nil
	})
	defer restore()

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

	var gotUserID string
	recordingEnforcer := func(ctx *gin.Context, rule access.AccessRule, uid string, r []string) bool {
		gotUserID = uid
		return true
	}
	access.RegisterAccessEnforcer(access.EntityOwner, recordingEnforcer)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	apiGroup := r.Group("/api/v1")
	// AuthManagement runs before Layer2Enforcement — userId is pre-set.
	apiGroup.Use(fakeAuthManagement(userID, presetRoles))
	apiGroup.Use(access.Layer2Enforcement())
	subGroup := apiGroup.Group("/test")
	subGroup.POST("/owned-resource/:id", okHandler)

	req := httptest.NewRequest(routeMethod, "/api/v1/test/owned-resource/abc-123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, int32(0), stubCalls.callCount(),
		"resolver MUST NOT be invoked when userId is already in the context")
	assert.Equal(t, userID, gotUserID,
		"access enforcer must receive the userId that AuthManagement set")
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestLayer2Enforcement_FallsThroughOnEmptyUserIdFromJWT covers the defensive
// branch in resolveUserForEnforcement that rejects an empty userID even when
// the JWT parsed without error. A malformed / malicious JWT whose `Id` claim
// is the empty string must NOT be treated as a valid authenticated identity:
// the enforcer must NOT run (which would silently allow the request through
// under permissive rules), and the middleware must pass through so
// downstream AuthManagement can reject with 401.
func TestLayer2Enforcement_FallsThroughOnEmptyUserIdFromJWT(t *testing.T) {
	const (
		routePath   = "/api/v1/test/owned-resource/:id"
		routeMethod = "POST"
	)

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	// Resolver returns no error but an empty userID — e.g. a JWT that parsed
	// successfully but whose Id claim is missing.
	restore := access.SetUserResolver(func(ctx *gin.Context) (string, []string, error) {
		return "", nil, nil
	})
	defer restore()

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
	apiGroup := r.Group("/api/v1")
	apiGroup.Use(access.Layer2Enforcement())
	subGroup := apiGroup.Group("/test")
	subGroup.POST("/owned-resource/:id", okHandler)

	req := httptest.NewRequest(routeMethod, "/api/v1/test/owned-resource/abc-123", nil)
	req.Header.Set("Authorization", "Bearer jwt-with-empty-userid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.False(t, spy.wasCalled(),
		"enforcer MUST NOT run when the resolved userID is empty — empty-identity JWTs must never reach Layer 2 handlers")
	assert.Equal(t, http.StatusOK, w.Code,
		"pass-through is the correct behaviour when userID is empty; downstream AuthManagement will reject with 401")
}

// TestLayer2Enforcement_AdminRoleResolvedViaJWT_BypassesEntityOwner asserts
// that admin bypass works end-to-end even when Layer2Enforcement resolves
// the userId from the JWT itself (i.e. roles come from the resolver, not
// from the context). The built-in EntityOwner enforcer checks IsAdmin(roles)
// first; if roles were lost during self-resolution, admin users would be
// incorrectly blocked.
func TestLayer2Enforcement_AdminRoleResolvedViaJWT_BypassesEntityOwner(t *testing.T) {
	const (
		routePath   = "/api/v1/test/owned-resource/:id"
		routeMethod = "POST"
		adminUserID = "platform-admin"
	)
	adminRoles := []string{"administrator"}

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	restore := access.SetUserResolver(func(ctx *gin.Context) (string, []string, error) {
		return adminUserID, adminRoles, nil
	})
	defer restore()

	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:   routePath,
		Method: routeMethod,
		Role:   "administrator",
		Access: access.AccessRule{
			Type:   access.EntityOwner,
			Entity: "TestEntity",
			Field:  "UserID",
		},
	})

	// Register a denying-by-default EntityOwner enforcer that admin-bypasses.
	// This mirrors the built-in behaviour without requiring an EntityLoader
	// stub — we only need to verify that admin roles resolved via the JWT
	// reach the enforcer.
	access.RegisterAccessEnforcer(access.EntityOwner, func(ctx *gin.Context, rule access.AccessRule, uid string, r []string) bool {
		if access.IsAdmin(r) {
			return true
		}
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"detail": "not admin and not owner"})
		return false
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	apiGroup := r.Group("/api/v1")
	apiGroup.Use(access.Layer2Enforcement())
	subGroup := apiGroup.Group("/test")
	subGroup.POST("/owned-resource/:id", okHandler)

	req := httptest.NewRequest(routeMethod, "/api/v1/test/owned-resource/abc-123", nil)
	req.Header.Set("Authorization", "Bearer admin-jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"admin role resolved via JWT must bypass EntityOwner rule")
}
