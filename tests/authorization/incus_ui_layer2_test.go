package authorization_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// ---------------------------------------------------------------------------
// MR B — dead Layer 2 rule on the Incus UI proxy route (H3)
// ---------------------------------------------------------------------------
//
// The Incus UI proxy is mounted as:
//
//	router.Group("/incus-ui").Any("/:backendId/*path", incusCookieAuth(), AuthManagement(), ProxyIncusUI)
//	(src/terminalTrainer/routes/terminalRoutes.go)
//
// For that route gin's ctx.FullPath() is:
//
//	/api/v1/incus-ui/:backendId/*path
//
// But the Layer 2 declaration in permissions.go registers the path WITHOUT
// the `path` name on the catch-all segment:
//
//	/api/v1/incus-ui/:backendId/*
//
// RouteRegistry.Lookup does an EXACT string match on "METHOD:path", so
// Layer2Enforcement() calls Lookup("GET", "/api/v1/incus-ui/:backendId/*path"),
// gets found == false, and passes the request through with NO Layer 2 check.
// The declared OrgRole rule is dead — every authenticated member reaches the
// proxy handler and relies solely on the controller's own predicate.
//
// Additionally, the declared rule uses OrgRole with Param "backendId". That is
// wrong: OrgRole treats the path parameter as an organization ID and calls
// CheckOrgRole(backendId, ...), but backendId is a backend identifier, not an
// org UUID, so the rule can never authorize the legitimate org manager it is
// meant to gate. The fix replaces OrgRole with a custom AccessRuleType whose
// enforcer delegates to IncusUIController.IsUserAuthorizedForBackend.
//
// This test pins BOTH halves of the bug against the REAL module registration
// (RegisterTerminalPermissions), so it fails today and only goes green once
// the declared path is corrected to ".../:backendId/*path" AND the rule type
// is no longer OrgRole.
//
// It is intentionally a pure registry assertion (no gin router / enforcer
// wiring) so the RED is deterministic and independent of the not-yet-existing
// enforcer.

// incusUIProxyMethods mirrors the HTTP methods the proxy route is declared for
// in permissions.go (it is a `.Any(...)` route, split into per-method Layer 2
// declarations because the registry Lookup is exact-match on method).
var incusUIProxyMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// registerTerminalPermissionsForTest resets the shared RouteRegistry and runs
// the real terminal-module permission registration against a mock enforcer, so
// the RouteRegistry reflects exactly what production wires up at startup.
func registerTerminalPermissionsForTest(t *testing.T) {
	t.Helper()
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	terminalController.RegisterTerminalPermissions(mocks.NewMockEnforcer())
}

// TestIncusUIProxy_Layer2Rule_IsLookupableAtRealFullPath is the guaranteed RED.
//
// It asserts that after the real module registration, the Layer 2 rule for the
// Incus UI proxy is retrievable at the FullPath gin actually reports at request
// time (".../:backendId/*path"). Today the rule is registered at ".../:backendId/*",
// so Lookup returns found == false and this fails — proving the rule is dead.
func TestIncusUIProxy_Layer2Rule_IsLookupableAtRealFullPath(t *testing.T) {
	registerTerminalPermissionsForTest(t)

	// The path gin's ctx.FullPath() yields for the `.Any("/:backendId/*path")`
	// route mounted under /api/v1/incus-ui. This is the exact key
	// Layer2Enforcement() uses to look the rule up.
	const realFullPath = "/api/v1/incus-ui/:backendId/*path"

	for _, method := range incusUIProxyMethods {
		t.Run(method, func(t *testing.T) {
			_, found := access.RouteRegistry.Lookup(method, realFullPath)
			assert.True(t, found,
				"Layer 2 rule for the Incus UI proxy must be registered at the FullPath gin "+
					"reports at request time (%q). It is currently declared at "+
					"'/api/v1/incus-ui/:backendId/*' (missing the 'path' catch-all name), so "+
					"Layer2Enforcement().Lookup(%q, %q) returns found=false and the request "+
					"passes through with NO Layer 2 check.", realFullPath, method, realFullPath)
		})
	}
}

// TestIncusUIProxy_Layer2Rule_DoesNotUseOrgRole pins the second half of the bug:
// the rule must not be an OrgRole rule keyed on "backendId". OrgRole would call
// CheckOrgRole(backendId, ...), but backendId is not an org UUID, so the rule
// can never authorize the org manager it is meant to gate. The fix delegates to
// IsUserAuthorizedForBackend via a dedicated AccessRuleType instead.
//
// This assertion is meaningful only once the path is corrected (otherwise the
// lookup misses and there is no rule to inspect); the companion test above
// carries the primary RED, and this one keeps the test suite RED through a
// path-only fix that leaves the wrong rule type in place.
func TestIncusUIProxy_Layer2Rule_DoesNotUseOrgRole(t *testing.T) {
	registerTerminalPermissionsForTest(t)

	const realFullPath = "/api/v1/incus-ui/:backendId/*path"

	for _, method := range incusUIProxyMethods {
		t.Run(method, func(t *testing.T) {
			perm, found := access.RouteRegistry.Lookup(method, realFullPath)
			if !found {
				// The primary RED is asserted by the companion test; skip the
				// type check here rather than dereference a zero-value perm.
				t.Skipf("rule not registered at %q yet — see TestIncusUIProxy_Layer2Rule_IsLookupableAtRealFullPath", realFullPath)
			}
			assert.NotEqual(t, access.OrgRole, perm.Access.Type,
				"Incus UI proxy Layer 2 rule must NOT use OrgRole with Param 'backendId' — "+
					"backendId is a backend id, not an org UUID, so CheckOrgRole can never "+
					"authorize the legitimate org manager. Use a custom AccessRuleType that "+
					"delegates to IsUserAuthorizedForBackend.")
		})
	}
}
