package terminalTrainer_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// ---------------------------------------------------------------------------
// C2 (runtime) — the Incus UI Layer 2 enforcer must allow the "_auth" backend
// ---------------------------------------------------------------------------
//
// The frontend bootstraps its auth cookie by calling
// POST /api/v1/incus-ui/_auth/cookie. The proxy HANDLER (ProxyIncusUI) special-
// cases backendId == "_auth" and sets the cookie BEFORE any authorization check
// (incusUIController.go:116). That special case runs for ANY authenticated user.
//
// Now that the Layer 2 rule is live (MR B), the global Layer2Enforcement()
// middleware runs the IncusBackendAccess enforcer BEFORE the handler. The
// enforcer delegates to IsUserAuthorizedForBackend(userID, roles, "_auth"),
// which — for a non-admin with no org owning the "_auth" pseudo-backend —
// returns false and aborts 403. So Layer 2 now blocks the cookie bootstrap for
// every non-admin user, breaking Incus UI for exactly the org managers it is
// meant to serve.
//
// The enforcer and handler are designed to "share the handler's predicate"
// (terminalRoutes.go:152) so both enforce the same policy. The fix therefore
// belongs in that shared predicate, IsUserAuthorizedForBackend: it must return
// true for backendId == "_auth" regardless of role (the handler already treats
// "_auth" as a pre-auth special case). Placing the bypass only in the enforcer
// closure would diverge the two paths and leave this test red.
//
// Two runtime assertions below:
//  1. The shared predicate contract (the single source of truth for the fix).
//  2. An end-to-end drive through the REAL Layer2Enforcement() middleware wired
//     to the same predicate, proving a non-admin reaches the handler (not 403).

// TestIncusUIAuthBypass_Predicate_AllowsAuthBackendForNonAdmin pins the shared
// predicate contract. This is the deterministic RED: a non-admin user with no
// org membership is currently denied "_auth" and must instead be allowed.
func TestIncusUIAuthBypass_Predicate_AllowsAuthBackendForNonAdmin(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	controller := terminalController.NewIncusUIController(db, "http://unused", nil)

	authorized := controller.IsUserAuthorizedForBackend(
		"non-admin-user",
		[]string{"member"},
		"_auth",
	)

	assert.True(t, authorized,
		"IsUserAuthorizedForBackend must allow the reserved '_auth' backend for any "+
			"authenticated user — the frontend bootstraps its auth cookie via "+
			"POST /api/v1/incus-ui/_auth/cookie, which ProxyIncusUI special-cases before "+
			"authorization. The shared Layer 2 enforcer delegates to this predicate, so a "+
			"false here means Layer2Enforcement 403s the cookie bootstrap for every non-admin.")
}

// TestIncusUIAuthBypass_Layer2Enforcement_AllowsAuthBackendForNonAdmin drives a
// non-admin request for the "_auth" backend through the REAL Layer2Enforcement()
// middleware, wired to the IncusBackendAccess enforcer exactly as production does
// (terminalRoutes.go:155), and asserts it reaches the handler rather than 403.
func TestIncusUIAuthBypass_Layer2Enforcement_AllowsAuthBackendForNonAdmin(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	controller := terminalController.NewIncusUIController(db, "http://unused", nil)

	// Register the IncusBackendAccess enforcer the same way TerminalRoutes does:
	// delegate to the shared predicate, abort 403 on denial. This mirrors
	// production so the test tracks a fix made in the shared predicate.
	access.RegisterAccessEnforcer(terminalController.IncusBackendAccess,
		func(ctx *gin.Context, rule access.AccessRule, userID string, roles []string) bool {
			backendID := ctx.Param(rule.Param)
			if controller.IsUserAuthorizedForBackend(userID, roles, backendID) {
				return true
			}
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return false
		})

	// Register the real Layer 2 rule (and siblings) via the module registration.
	terminalController.RegisterTerminalPermissions(mocks.NewMockEnforcer())

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "non-admin-user")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.Use(access.Layer2Enforcement())
	// Dummy handler isolates the middleware: a 200 here proves Layer 2 allowed
	// the request (the real ProxyIncusUI is not involved).
	router.POST("/api/v1/incus-ui/:backendId/*path", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "reached handler"})
	})

	req := httptest.NewRequest("POST", "/api/v1/incus-ui/_auth/cookie", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Layer2Enforcement must not 403 a non-admin bootstrapping the Incus UI auth cookie "+
			"(POST /api/v1/incus-ui/_auth/cookie). The IncusBackendAccess enforcer delegates to "+
			"IsUserAuthorizedForBackend, which must allow the reserved '_auth' backend.")
	assert.Equal(t, http.StatusOK, w.Code,
		"non-admin '_auth' request should reach the handler")
}
