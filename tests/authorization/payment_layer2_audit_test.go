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
//
// Scenarios 1–4 are covered by the generic parameterized suite in
// layer2_audit_parameterized_test.go via adaptPaymentRoutes().
// This file retains only the route catalog and scenario 5 (NoUser_Passthrough),
// which is specific to the payment module and cannot be generalized without
// adding complexity to the shared suite.

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		route := route // capture
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			// Reuse the generic helpers from layer2_audit_parameterized_test.go.
			adapted := adaptPaymentRoute(route)
			checker := &layer2AuditMembershipChecker{orgRoles: map[string]string{}}
			loader := &layer2AuditEntityLoader{owners: map[string]string{}}
			r := setupLayer2AuditRouter(t, adapted, checker, loader)

			// No X-Test-UserId header → userId="" → Layer 2 passthrough.
			w := doLayer2AuditRequest(r, route.method, route.requestPath, "", "")
			assert.Equal(t, http.StatusOK, w.Code,
				"Layer 2 must pass through when no user is resolved (AuthManagement owns the 401 response) on %s %s", route.method, route.requestPath)
		})
	}
}

// adaptPaymentRoute converts a single paymentAuditRoute to layer2AuditRoute
// for use with the generic helpers.
func adaptPaymentRoute(r paymentAuditRoute) layer2AuditRoute {
	return adaptPaymentRoutes()[findPaymentRouteIndex(r)]
}

// findPaymentRouteIndex returns the index of r in paymentAuditRoutes so we can
// use the pre-built adapted slice from adaptPaymentRoutes() without duplicating
// the conversion logic.
func findPaymentRouteIndex(r paymentAuditRoute) int {
	for i, pr := range paymentAuditRoutes {
		if pr.method == r.method && pr.registeredPath == r.registeredPath {
			return i
		}
	}
	return 0
}
