package authorization_tests

import (
	"strings"
	"testing"

	"github.com/casbin/casbin/v2/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
// the RouteRegistry reflects exactly what production wires up at startup. The
// mock enforcer is returned so callers can inspect the Layer 1 Casbin policies
// the module registered via ReconcilePolicy (recorded as AddPolicy calls).
func registerTerminalPermissionsForTest(t *testing.T) *mocks.MockEnforcer {
	t.Helper()
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	enforcer := mocks.NewMockEnforcer()
	terminalController.RegisterTerminalPermissions(enforcer)
	return enforcer
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

// ---------------------------------------------------------------------------
// C1 (runtime) — Layer 1 Casbin policy path must match concrete request URLs
// ---------------------------------------------------------------------------
//
// The prior shape-only tests assert the Layer 2 registry key (which is matched
// against gin's FullPath ".../:backendId/*path" by exact string equality). But
// the SAME module also registers a Layer 1 Casbin policy for the proxy via
// ReconcilePolicy (permissions.go). Casbin's AuthManagement matches that policy
// against the CONCRETE request URL (ctx.Request.URL.Path) using keyMatch2 — NOT
// FullPath, NOT exact match.
//
// keyMatch2 only treats "/*" as a wildcard: it rewrites "/*" → "/.*". So a
// policy path ending in "/*path" becomes the regex ".../.*path$", which forces
// every request URL to END WITH the literal string "path". Real Incus UI URLs
// (".../ui/assets/app.js", ".../1.0/instances", ".../_auth/cookie") do not, so
// Casbin denies them with 403 for non-admins — the "missing Authorization
// header" symptom.
//
// The two path formats are therefore DIFFERENT on purpose:
//   - Layer 2 registry key : /api/v1/incus-ui/:backendId/*path  (exact vs FullPath)
//   - Layer 1 Casbin policy: /api/v1/incus-ui/:backendId/*      (keyMatch2 vs URL)
//
// This test reads the ACTUAL Layer 1 policy path the module registers (captured
// via the mock enforcer's AddPolicy calls) and asserts keyMatch2 matches real
// URLs. It fails while the registered policy is ".../:backendId/*path" and
// passes once it is reverted to ".../:backendId/*". The existing
// incusUICasbinPolicy_test.go proves keyMatch2 semantics on HARD-CODED strings;
// it cannot catch a regression in what the module actually registers — this does.

// registeredIncusUILayer1Paths returns every Casbin policy path the terminal
// module registers for the Incus UI proxy (each AddPolicy call whose path is
// under /api/v1/incus-ui/). Since MR N derives Layer 1 from the RouteRegistry,
// the proxy now registers one row per HTTP method (GET/POST/PUT/PATCH/DELETE)
// rather than a single "(GET|POST|PUT|PATCH|DELETE)" regex row — so this returns
// all five. Fails the test if none are found.
func registeredIncusUILayer1Paths(t *testing.T, enforcer *mocks.MockEnforcer) []string {
	t.Helper()
	var found []string
	for _, call := range enforcer.AddPolicyCalls {
		// ReconcilePolicy calls AddPolicy(role, path, method); path is arg 1.
		if len(call) < 2 {
			continue
		}
		path, ok := call[1].(string)
		if !ok {
			continue
		}
		if strings.HasPrefix(path, "/api/v1/incus-ui/") {
			found = append(found, path)
		}
	}
	require.Len(t, found, 5,
		"expected exactly five Incus UI Layer 1 Casbin policies (one per HTTP method) "+
			"to be registered, got %v", found)
	return found
}

// TestIncusUIProxy_Layer1Policy_MatchesConcreteURLs is the C1 runtime guard.
//
// MR N derives Layer 1 from the RouteRegistry, so the proxy now registers five
// concrete per-method Casbin rows instead of one regex-method row. The core
// security property is unchanged and asserted against EVERY row: each Layer 1
// policy path must keyMatch2 the real, concrete Incus UI request URLs. If any
// row carried the registry's /*path form instead of the CasbinPath /* override,
// keyMatch2 would fail on real URLs and non-admins would be denied 403 (the MR B
// bug). Running the check over all five rows proves the override applied to each.
func TestIncusUIProxy_Layer1Policy_MatchesConcreteURLs(t *testing.T) {
	enforcer := registerTerminalPermissionsForTest(t)
	policyPaths := registeredIncusUILayer1Paths(t, enforcer)

	// Every registered incus-ui Layer 1 path must be the keyMatch2 wildcard form
	// (/*), never the registry's literal-suffix /*path form.
	for _, p := range policyPaths {
		assert.Equal(t, "/api/v1/incus-ui/:backendId/*", p,
			"every Incus UI Layer 1 Casbin policy path must be the keyMatch2 '/*' form "+
				"(from the CasbinPath override), never the registry '/*path' form; got %q", p)
	}

	// Concrete request URLs (ctx.Request.URL.Path) that a real Incus UI session
	// generates. None of these end with the literal "path", so a "/*path"
	// policy fails keyMatch2 for all of them.
	realURLs := []string{
		"/api/v1/incus-ui/_auth/cookie",                       // cookie bootstrap (non-admin, POST)
		"/api/v1/incus-ui/backend123/ui/",                     // iframe initial load
		"/api/v1/incus-ui/backend123/ui/assets/app.js",        // static asset
		"/api/v1/incus-ui/backend123/1.0/instances",           // Incus API call
		"/api/v1/incus-ui/backend123/1.0/instances/foo/state", // deep nested API path
	}

	// The security property must hold for EVERY registered per-method row.
	for _, policyPath := range policyPaths {
		for _, url := range realURLs {
			t.Run(policyPath+" vs "+url, func(t *testing.T) {
				assert.True(t, util.KeyMatch2(url, policyPath),
					"Layer 1 Casbin policy %q must keyMatch2 the concrete request URL %q. "+
						"AuthManagement matches Casbin policies against ctx.Request.URL.Path with "+
						"keyMatch2, which rewrites '/*' → '/.*' only — a '/*path' suffix becomes "+
						"'/.*path$' and requires the URL to end with the literal 'path', so every "+
						"real Incus UI request is denied 403. Register the Layer 1 policy path as "+
						"'/api/v1/incus-ui/:backendId/*' (Layer 2 keeps '*path' for the exact "+
						"FullPath match).", policyPath, url)
			})
		}
	}
}
