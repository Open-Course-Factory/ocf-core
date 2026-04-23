package authorization_tests

// Payment module Layer 2 authorization audit (#263).
//
// Verifies that the five OrgRole-protected payment routes declared in
// src/payment/routes/permissions.go are actually enforced end-to-end by the
// Layer2Enforcement middleware. For each route we run five scenarios:
//
//   1. Outsider — Casbin Member with NO organization membership → expect 403
//   2. Insufficient role — user is an org "member" but route requires "manager" → expect 403
//   3. Authorized — user meets the declared MinRole → expect 200 (fake handler)
//   4. Admin bypass — Casbin Administrator → expect 200 (fake handler)
//   5. No JWT / no userId in context → Layer 2 passes through to the next
//      middleware (AuthManagement in production would reject with 401). In
//      this harness there is no AuthManagement, so "passthrough" is verified
//      as a 200 from the fake handler, and the test asserts the request was
//      NOT blocked by Layer 2.
//
// The fake handler is a no-op that returns 200. If Layer 2 allows the request
// through, the handler fires and we see 200. If Layer 2 blocks, we see 403.
// This avoids needing to stand up the real Stripe-backed controller.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
)

// paymentAuditMembershipChecker is a local MembershipChecker for the payment
// audit. It records what roles each user has in each org and answers
// CheckOrgRole based on a provided role map. We use a distinct type (not the
// shared mockMembershipChecker) to avoid coupling this audit to tests in
// enforcement_middleware_test.go.
type paymentAuditMembershipChecker struct {
	// orgRoles maps "orgID:userID" -> role name (e.g. "member", "manager",
	// "owner"). Absence means the user has no membership in that org.
	orgRoles map[string]string
}

func (c *paymentAuditMembershipChecker) CheckGroupRole(groupID, userID, minRole string) (bool, error) {
	return false, nil
}

func (c *paymentAuditMembershipChecker) CheckOrgRole(orgID, userID, minRole string) (bool, error) {
	role, ok := c.orgRoles[orgID+":"+userID]
	if !ok {
		return false, nil
	}
	return access.IsRoleAtLeast(role, minRole), nil
}

// paymentAuditEntityLoader is unused by OrgRole but required by the
// RegisterBuiltinEnforcers signature.
type paymentAuditEntityLoader struct{}

func (l *paymentAuditEntityLoader) GetOwnerField(entity, id, field string) (string, error) {
	return "", nil
}

// paymentAuditRoute describes one of the five routes under audit.
type paymentAuditRoute struct {
	// method is the HTTP verb.
	method string
	// registeredPath is the Gin route pattern (with :id).
	registeredPath string
	// requestPath is the concrete path used in the HTTP request.
	requestPath string
	// orgID is the value that will appear in :id for the request path.
	orgID string
	// minRole mirrors the RoutePermission.Access.MinRole declaration.
	minRole string
}

// paymentAuditRoutes enumerates the five OrgRole-protected payment routes
// declared in src/payment/routes/permissions.go (lines 115-152).
var paymentAuditRoutes = []paymentAuditRoute{
	{method: "POST", registeredPath: "/api/v1/organizations/:id/subscribe", requestPath: "/api/v1/organizations/org-audit-1/subscribe", orgID: "org-audit-1", minRole: "manager"},
	{method: "GET", registeredPath: "/api/v1/organizations/:id/subscription", requestPath: "/api/v1/organizations/org-audit-2/subscription", orgID: "org-audit-2", minRole: "member"},
	{method: "DELETE", registeredPath: "/api/v1/organizations/:id/subscription", requestPath: "/api/v1/organizations/org-audit-3/subscription", orgID: "org-audit-3", minRole: "manager"},
	{method: "GET", registeredPath: "/api/v1/organizations/:id/features", requestPath: "/api/v1/organizations/org-audit-4/features", orgID: "org-audit-4", minRole: "member"},
	{method: "GET", registeredPath: "/api/v1/organizations/:id/usage-limits", requestPath: "/api/v1/organizations/org-audit-5/usage-limits", orgID: "org-audit-5", minRole: "member"},
}

// setupPaymentAuditRouter sets up a Gin engine with:
//   - a header-driven userId/userRoles injector (matching setupTestRouter in
//     enforcement_middleware_test.go)
//   - Layer2Enforcement middleware
//   - a fake handler on the declared route that returns 200
//
// The membership checker is provided by the caller so each subtest can
// control what org roles each user has.
func setupPaymentAuditRouter(t *testing.T, route paymentAuditRoute, checker access.MembershipChecker) *gin.Engine {
	t.Helper()

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	// Register the real route permission exactly as declared in
	// src/payment/routes/permissions.go.
	access.RouteRegistry.Register("Organization Subscriptions",
		access.RoutePermission{
			Path:   route.registeredPath,
			Method: route.method,
			Role:   "member",
			Access: access.AccessRule{
				Type:    access.OrgRole,
				Param:   "id",
				MinRole: route.minRole,
			},
		},
	)

	loader := &paymentAuditEntityLoader{}
	access.RegisterBuiltinEnforcers(loader, checker)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Inject userId / userRoles from headers BEFORE Layer 2 — the same
	// pattern used by setupTestRouter in enforcement_middleware_test.go.
	// If X-Test-UserId is empty (unauthenticated case) we deliberately set
	// an empty string so resolveUserForEnforcement returns ok=false.
	r.Use(func(c *gin.Context) {
		uid := c.GetHeader("X-Test-UserId")
		c.Set("userId", uid)
		rolesHeader := c.GetHeader("X-Test-Roles")
		if rolesHeader != "" {
			c.Set("userRoles", strings.Split(rolesHeader, ","))
		} else {
			c.Set("userRoles", []string{})
		}
		c.Next()
	})
	r.Use(access.Layer2Enforcement())

	// Fake handler — if the request reaches here, Layer 2 allowed it.
	fake := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "layer2-allowed"})
	}
	r.Handle(route.method, route.registeredPath, fake)
	return r
}

// doRequest performs the HTTP request with the given userId + roles headers.
func doRequest(r *gin.Engine, method, path, userID, roles string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if userID != "" {
		req.Header.Set("X-Test-UserId", userID)
	}
	if roles != "" {
		req.Header.Set("X-Test-Roles", roles)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// -----------------------------------------------------------------------------
// Case 1: Outsider — Member with no org membership at all → 403
// -----------------------------------------------------------------------------

func TestPaymentLayer2_Outsider_Denied(t *testing.T) {
	for _, route := range paymentAuditRoutes {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			// Checker has NO entries for this user in this org.
			checker := &paymentAuditMembershipChecker{orgRoles: map[string]string{}}
			r := setupPaymentAuditRouter(t, route, checker)

			w := doRequest(r, route.method, route.requestPath, "outsider-user", "member")
			assert.Equal(t, http.StatusForbidden, w.Code,
				"outsider (no org membership) must be denied on %s %s", route.method, route.requestPath)
		})
	}
}

// -----------------------------------------------------------------------------
// Case 2: Insufficient role — user is a plain org "member" but route needs
// "manager". Only applies to the three manager-gated routes.
// -----------------------------------------------------------------------------

func TestPaymentLayer2_InsufficientRole_Denied(t *testing.T) {
	for _, route := range paymentAuditRoutes {
		if route.minRole != "manager" {
			continue // only manager-gated routes have an insufficient-role case
		}
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			checker := &paymentAuditMembershipChecker{orgRoles: map[string]string{
				route.orgID + ":insufficient-user": "member",
			}}
			r := setupPaymentAuditRouter(t, route, checker)

			w := doRequest(r, route.method, route.requestPath, "insufficient-user", "member")
			assert.Equal(t, http.StatusForbidden, w.Code,
				"org member must be denied on manager-gated %s %s", route.method, route.requestPath)
		})
	}
}

// -----------------------------------------------------------------------------
// Case 3: Authorized — user meets MinRole → Layer 2 lets the request through
// (fake handler returns 200).
// -----------------------------------------------------------------------------

func TestPaymentLayer2_Authorized_Allowed(t *testing.T) {
	for _, route := range paymentAuditRoutes {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			// Grant exactly the minimum required role.
			checker := &paymentAuditMembershipChecker{orgRoles: map[string]string{
				route.orgID + ":authorized-user": route.minRole,
			}}
			r := setupPaymentAuditRouter(t, route, checker)

			w := doRequest(r, route.method, route.requestPath, "authorized-user", "member")
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"user with MinRole=%s must not be blocked by Layer 2 on %s %s", route.minRole, route.method, route.requestPath)
			assert.Equal(t, http.StatusOK, w.Code,
				"fake handler should have returned 200 for %s %s", route.method, route.requestPath)
		})
	}
}

// -----------------------------------------------------------------------------
// Case 4: Admin bypass — Casbin Administrator → 200 even without org
// membership.
// -----------------------------------------------------------------------------

func TestPaymentLayer2_AdminBypass_Allowed(t *testing.T) {
	for _, route := range paymentAuditRoutes {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			// Admin has NO org membership — bypass must still allow.
			checker := &paymentAuditMembershipChecker{orgRoles: map[string]string{}}
			r := setupPaymentAuditRouter(t, route, checker)

			w := doRequest(r, route.method, route.requestPath, "admin-user", "administrator")
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"administrator must bypass OrgRole check on %s %s", route.method, route.requestPath)
			assert.Equal(t, http.StatusOK, w.Code,
				"admin must be allowed on %s %s", route.method, route.requestPath)
		})
	}
}

// -----------------------------------------------------------------------------
// Case 5: No JWT / unauthenticated — Layer2Enforcement deliberately passes
// through so AuthManagement (which runs later in the real chain) can reject
// with 401. In this harness there is no AuthManagement, so passthrough means
// the fake handler fires. We assert this explicitly to guard against a future
// change that would let Layer 2 silently enforce authorization when the user
// is unresolved (which would mask a bug elsewhere).
//
// NOTE: In production the full chain is CORS -> AuthManagement -> Layer2 ->
// handler, so unauthenticated requests get 401 from AuthManagement before
// Layer 2 runs. This test documents the intended Layer 2 passthrough
// semantics, not a real 401 path.
// -----------------------------------------------------------------------------

func TestPaymentLayer2_NoUser_Passthrough(t *testing.T) {
	for _, route := range paymentAuditRoutes {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			checker := &paymentAuditMembershipChecker{orgRoles: map[string]string{}}
			r := setupPaymentAuditRouter(t, route, checker)

			// No X-Test-UserId header → userId="" → Layer 2 passthrough.
			w := doRequest(r, route.method, route.requestPath, "", "")
			assert.Equal(t, http.StatusOK, w.Code,
				"Layer 2 must pass through when no user is resolved (AuthManagement owns the 401 response) on %s %s", route.method, route.requestPath)
		})
	}
}
