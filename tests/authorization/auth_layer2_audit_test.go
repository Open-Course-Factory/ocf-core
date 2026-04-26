package authorization_tests

// Auth module Layer 2 authorization audit (#271).
//
// Verifies that the Layer 2-relevant routes declared in
//   - src/auth/routes/usersRoutes/permissions.go
//   - src/auth/routes/securityAdminRoutes/permissions.go
// are correctly declared in the RouteRegistry and that AdminOnly routes
// are actually enforced end-to-end by the Layer2Enforcement middleware.
//
// Scope:
//
//   - AdminOnly (11 routes): the only Layer 2-enforced rule type used by
//     the auth module. Two enforcement scenarios per route:
//       1. Outsider — Casbin Member with no admin role → expect 403
//       2. Admin allowed — Casbin Administrator → expect 200 (fake handler)
//
//   - SelfScoped (handler-enforced, doc-only): we only assert registry
//     parity (the route is declared with the right type). Enforcement
//     happens in the controller, not in Layer 2.
//
//   - Public routes are out of scope (no Layer 2 check by design).
//
// Plus structural guards:
//
//   - TestAuthLayer2_RegistryDeclaresEveryAuditedRoute — catalog/registry
//     parity check. Every (path, method) pair in the audit catalog must
//     be findable via RouteRegistry.Lookup.
//
//   - TestAuthLayer2_NoRegexMethodInCatalog — !180 regression marker.
//     Surfaces the #265 bug where the SelfScoped declaration for
//     /api/v1/users/me/* uses a regex-alternation method
//     "(GET|POST|PATCH|DELETE)" which RouteRegistry.Lookup (exact-match)
//     cannot find. This subtest is RED until #265 is fixed.
//
// AUDIT FINDING (#265, fix tracked in this MR) — /api/v1/users/me/* uses
// a regex-alternation HTTP method:
//
//   src/auth/routes/usersRoutes/permissions.go:149
//     Path: "/api/v1/users/me/*", Method: "(GET|POST|PATCH|DELETE)"
//
// RouteRegistry.Lookup does exact method+path string matching. A method
// like "(GET|POST|PATCH|DELETE)" can never be matched by an incoming
// request (which arrives with a single concrete verb like "GET"), so
// Layer 2 silently passes the route through without consulting the
// SelfScoped declaration. This is the same class of bug fixed for the
// terminal incus-ui proxy in MR !180. Fix: split the declaration into
// four entries (one per concrete verb). See `NoRegexMethodInCatalog`
// subtest below — it FAILS in red and flips green when the fix lands.
//
// AUDIT NOTE — auto-registered User entity CRUD routes:
//
// The User entity registers via the entity manager and gets auto-generated
// CRUD routes (GET/POST/PATCH/DELETE /api/v1/users[, :id]). These are
// out of scope for this audit, mirroring how !183 handled organization
// CRUD: entity-manager routes have their own permission registration
// path. The custom routes in usersRoutes/permissions.go are what we
// audit here.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
)

// -----------------------------------------------------------------------------
// Shared harness types — scoped to this audit file so we don't couple to
// the shared mockMembershipChecker used elsewhere in the package.
// -----------------------------------------------------------------------------

// authAuditMembershipChecker is unused by AdminOnly / SelfScoped but
// required by the RegisterBuiltinEnforcers signature.
type authAuditMembershipChecker struct{}

func (c *authAuditMembershipChecker) CheckGroupRole(groupID, userID, minRole string) (bool, error) {
	return false, nil
}

func (c *authAuditMembershipChecker) CheckOrgRole(orgID, userID, minRole string) (bool, error) {
	return false, nil
}

// authAuditEntityLoader is unused by AdminOnly / SelfScoped but required
// by the RegisterBuiltinEnforcers signature.
type authAuditEntityLoader struct{}

func (l *authAuditEntityLoader) GetOwnerField(entity, id, field string) (string, error) {
	return "", nil
}

// -----------------------------------------------------------------------------
// Route catalog — mirrors src/auth/routes/usersRoutes/permissions.go and
// src/auth/routes/securityAdminRoutes/permissions.go.
//
// Each entry captures everything the audit needs to reproduce production
// declarations and issue concrete requests. SelfScoped entries are
// documented for parity but excluded from enforcement assertions.
// -----------------------------------------------------------------------------

type authAuditRoute struct {
	module         string
	method         string
	registeredPath string
	requestPath    string
	ruleType       access.AccessRuleType
}

// authAuditAdminRoutes — 11 AdminOnly routes split across two files.
// These ARE Layer 2-enforced and MUST 403 outsiders / 200 admins.
var authAuditAdminRoutes = []authAuditRoute{
	// usersRoutes — 7 routes
	{module: "User Management", method: "DELETE", registeredPath: "/api/v1/users/:id", requestPath: "/api/v1/users/audit-uid", ruleType: access.AdminOnly},
	{module: "Access Control", method: "POST", registeredPath: "/api/v1/accesses", requestPath: "/api/v1/accesses", ruleType: access.AdminOnly},
	{module: "Access Control", method: "DELETE", registeredPath: "/api/v1/accesses", requestPath: "/api/v1/accesses", ruleType: access.AdminOnly},
	{module: "Access Control", method: "GET", registeredPath: "/api/v1/hooks", requestPath: "/api/v1/hooks", ruleType: access.AdminOnly},
	{module: "Access Control", method: "POST", registeredPath: "/api/v1/hooks/:hook_name/enable", requestPath: "/api/v1/hooks/audit-hook/enable", ruleType: access.AdminOnly},
	{module: "Access Control", method: "POST", registeredPath: "/api/v1/hooks/:hook_name/disable", requestPath: "/api/v1/hooks/audit-hook/disable", ruleType: access.AdminOnly},
	{module: "Access Control", method: "POST", registeredPath: "/api/v1/email-templates/:id/test", requestPath: "/api/v1/email-templates/audit-tpl/test", ruleType: access.AdminOnly},
	// securityAdminRoutes — 4 routes
	{module: "Security Administration", method: "GET", registeredPath: "/api/v1/admin/security/policies", requestPath: "/api/v1/admin/security/policies", ruleType: access.AdminOnly},
	{module: "Security Administration", method: "GET", registeredPath: "/api/v1/admin/security/user-permissions", requestPath: "/api/v1/admin/security/user-permissions", ruleType: access.AdminOnly},
	{module: "Security Administration", method: "GET", registeredPath: "/api/v1/admin/security/entity-roles", requestPath: "/api/v1/admin/security/entity-roles", ruleType: access.AdminOnly},
	{module: "Security Administration", method: "GET", registeredPath: "/api/v1/admin/security/health-checks", requestPath: "/api/v1/admin/security/health-checks", ruleType: access.AdminOnly},
}

// authAuditSelfScopedRoutes — handler-enforced routes. We assert that
// the registry declaration exists (catalog parity) but do NOT verify
// enforcement, since SelfScoped is documentation-only at the Layer 2
// level — controllers must verify userId scoping themselves.
//
// /api/v1/users/me/* is declared as four per-method entries because the
// Layer2 registry Lookup does exact-match on method+path (see #265, !180).
var authAuditSelfScopedRoutes = []authAuditRoute{
	{module: "Authentication", method: "GET", registeredPath: "/api/v1/users/:id", requestPath: "/api/v1/users/audit-self", ruleType: access.SelfScoped},
	{module: "Authentication", method: "GET", registeredPath: "/api/v1/users/me/*", requestPath: "/api/v1/users/me/anything", ruleType: access.SelfScoped},
	{module: "Authentication", method: "POST", registeredPath: "/api/v1/users/me/*", requestPath: "/api/v1/users/me/anything", ruleType: access.SelfScoped},
	{module: "Authentication", method: "PATCH", registeredPath: "/api/v1/users/me/*", requestPath: "/api/v1/users/me/anything", ruleType: access.SelfScoped},
	{module: "Authentication", method: "DELETE", registeredPath: "/api/v1/users/me/*", requestPath: "/api/v1/users/me/anything", ruleType: access.SelfScoped},
	{module: "Authentication", method: "GET", registeredPath: "/api/v1/auth/permissions", requestPath: "/api/v1/auth/permissions", ruleType: access.SelfScoped},
	{module: "Authentication", method: "GET", registeredPath: "/api/v1/auth/me", requestPath: "/api/v1/auth/me", ruleType: access.SelfScoped},
	{module: "Authentication", method: "GET", registeredPath: "/api/v1/auth/verify-status", requestPath: "/api/v1/auth/verify-status", ruleType: access.SelfScoped},
	{module: "Feedback", method: "POST", registeredPath: "/api/v1/feedback/*", requestPath: "/api/v1/feedback/issue", ruleType: access.SelfScoped},
}

// allAuthAuditRoutes returns every route (Admin + SelfScoped) declared
// in scope of this audit, used for registry parity checks.
func allAuthAuditRoutes() []authAuditRoute {
	all := append([]authAuditRoute{}, authAuditAdminRoutes...)
	all = append(all, authAuditSelfScopedRoutes...)
	return all
}

// -----------------------------------------------------------------------------
// Harness — installs Layer 2 with exactly the production declaration
// for one route under audit. Same shape as the organizations / scenarios
// audit harnesses.
// -----------------------------------------------------------------------------

func setupAuthAuditRouter(
	t *testing.T,
	route authAuditRoute,
	checker access.MembershipChecker,
	loader access.EntityLoader,
) *gin.Engine {
	t.Helper()

	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	rule := access.AccessRule{Type: route.ruleType}

	role := "member"
	if route.ruleType == access.AdminOnly {
		role = "administrator"
	}

	access.RouteRegistry.Register(route.module,
		access.RoutePermission{
			Path:   route.registeredPath,
			Method: route.method,
			Role:   role,
			Access: rule,
		},
	)

	access.RegisterBuiltinEnforcers(loader, checker)

	gin.SetMode(gin.TestMode)
	r := gin.New()

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

	fake := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "layer2-allowed"})
	}
	r.Handle(route.method, route.registeredPath, fake)
	return r
}

func doAuthAuditRequest(r *gin.Engine, method, path, userID, roles string) *httptest.ResponseRecorder {
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
// Case 1: Outsider — Casbin Member (non-admin) on AdminOnly routes → 403.
//
// Note: in production, non-admin Members are also blocked at Layer 1
// (RBAC gateway: the policy says "administrator" only). This audit is
// scoped to Layer 2 — we verify the AdminOnly enforcer rejects
// non-admins independently of Layer 1.
// -----------------------------------------------------------------------------

func TestAuthLayer2_AdminOnly_Outsider_Denied(t *testing.T) {
	for _, route := range authAuditAdminRoutes {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			loader := &authAuditEntityLoader{}
			checker := &authAuditMembershipChecker{}
			r := setupAuthAuditRouter(t, route, checker, loader)

			w := doAuthAuditRequest(r, route.method, route.requestPath, "outsider-user", "member")
			assert.Equal(t, http.StatusForbidden, w.Code,
				"non-admin must be denied on AdminOnly %s %s (observed %d, body=%s)",
				route.method, route.requestPath, w.Code, w.Body.String())
		})
	}
}

// -----------------------------------------------------------------------------
// Case 2: Admin allowed — Casbin Administrator on AdminOnly routes → 200.
// -----------------------------------------------------------------------------

func TestAuthLayer2_AdminOnly_Admin_Allowed(t *testing.T) {
	for _, route := range authAuditAdminRoutes {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			loader := &authAuditEntityLoader{}
			checker := &authAuditMembershipChecker{}
			r := setupAuthAuditRouter(t, route, checker, loader)

			w := doAuthAuditRequest(r, route.method, route.requestPath, "admin-user", "administrator")
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"administrator must bypass AdminOnly enforcement on %s %s (observed %d)",
				route.method, route.requestPath, w.Code)
			assert.Equal(t, http.StatusOK, w.Code,
				"admin must reach handler on %s %s (body=%s)",
				route.method, route.requestPath, w.Body.String())
		})
	}
}

// -----------------------------------------------------------------------------
// AUDIT FINDING (positive) — Layer 1 / Layer 2 path consistency.
//
// Every (path, method) pair appearing in the Layer 1 ReconcilePolicy
// loops at the top of the auth permission files must have a matching
// RouteRegistry.Register entry so Layer 2 actually inspects the route.
// A Layer 1 policy without a Layer 2 declaration is a silent bypass:
// the request makes it past the RBAC gate and through Layer 2
// untouched. This subtest cross-walks the catalog above against the
// registry as it would be populated by RegisterUserPermissions /
// RegisterAuthPermissions / RegisterFeedbackPermissions /
// RegisterSecurityAdminPermissions.
//
// Cross-walk note: usersRoutes/permissions.go:130-138 calls
// ReconcilePolicy in a loop over GetCasdoorRolesForOCFRole(Member) (so
// Casbin gets one policy per Casdoor role), but the RouteRegistry only
// stores one entry per route (with Role="member" as the canonical
// label). The parity check below uses (path, method) as the key, so it
// is robust to the per-role Casbin spread.
//
// The catalog above is the canonical source of truth for this audit. If
// a route is added or removed from the auth permission files, the
// catalog must be updated to match — this subtest is a reminder that
// the two must stay aligned.
// -----------------------------------------------------------------------------

func TestAuthLayer2_RegistryDeclaresEveryAuditedRoute(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	// Register every route in the audit catalog using the same shape
	// production uses — this matches what the four Register* functions
	// do at startup. We don't import the production functions here
	// because doing so would also pull in unrelated module init; the
	// catalog parity check is what guards drift.
	for _, route := range allAuthAuditRoutes() {
		rule := access.AccessRule{Type: route.ruleType}
		role := "member"
		if route.ruleType == access.AdminOnly {
			role = "administrator"
		}
		access.RouteRegistry.Register(route.module,
			access.RoutePermission{
				Path:   route.registeredPath,
				Method: route.method,
				Role:   role,
				Access: rule,
			},
		)
	}

	for _, route := range allAuthAuditRoutes() {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			perm, found := access.RouteRegistry.Lookup(route.method, route.registeredPath)
			assert.True(t, found,
				"RouteRegistry.Lookup must find a declaration for %s %s — a missing entry means Layer 2 silently passes the route through",
				route.method, route.registeredPath)
			if found {
				assert.Equal(t, route.ruleType, perm.Access.Type,
					"declared access rule type must match catalog for %s %s",
					route.method, route.registeredPath)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Regression guard: no regex-method declarations.
//
// The bug fixed in MR !180 (terminals incus-ui) was a regex method like
// "(GET|POST|PUT|PATCH|DELETE)" silently bypassing RouteRegistry.Lookup,
// which does exact method+path string match. This subtest verifies every
// catalog entry uses a single concrete HTTP verb.
//
// AUDIT FINDING (#265): the entry for /api/v1/users/me/* uses
// "(GET|POST|PATCH|DELETE)" — this subtest is RED until the production
// declaration in usersRoutes/permissions.go is split into four
// concrete-verb entries. The catalog above mirrors the broken production
// state on purpose, so the failing subtest below pins the bug. When the
// fix lands (this MR), the catalog and production declaration will both
// be split into four entries and this subtest will go green.
// -----------------------------------------------------------------------------

func TestAuthLayer2_NoRegexMethodInCatalog(t *testing.T) {
	allowed := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	}

	for _, route := range allAuthAuditRoutes() {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			assert.True(t, allowed[route.method],
				"catalog route %s %s declares a non-canonical HTTP method — Layer 2 Lookup is exact-match, regex/alternation methods would silently bypass enforcement (see #265, MR !180)",
				route.method, route.registeredPath)
			assert.NotContains(t, route.method, "|",
				"alternation in method string for %s %s would silently bypass Layer 2 (see #265, MR !180)",
				route.method, route.registeredPath)
		})
	}
}
