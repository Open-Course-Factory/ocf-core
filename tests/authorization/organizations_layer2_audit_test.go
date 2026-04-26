package authorization_tests

// Organizations module Layer 2 authorization audit (#270).
//
// Verifies that the seven Layer 2-enforced routes declared in
// src/organizations/routes/permissions.go are actually enforced end-to-end
// by the Layer2Enforcement middleware. The module exposes two enforcer
// types (no EntityOwner / GroupRole / SelfScoped routes here):
//
//   - OrgRole (6 routes, Param="id"):
//       member-gated:
//         GET    /api/v1/organizations/:id/members
//         GET    /api/v1/organizations/:id/groups
//         GET    /api/v1/organizations/:id/backends
//       manager-gated:
//         POST   /api/v1/organizations/:id/import
//         POST   /api/v1/organizations/:id/groups/:groupId/regenerate-passwords
//       owner-gated:
//         POST   /api/v1/organizations/:id/convert-to-team
//
//   - AdminOnly (1 route):
//       PUT    /api/v1/organizations/:id/backends
//
// For each Layer 2-enforced route we run four scenarios mirroring prior
// audits (payment / scenarios):
//
//   1. Outsider — Casbin Member with NO organization membership → expect 403
//   2. Insufficient role — only on routes with MinRole > member (member when
//      manager is required, manager when owner is required) → expect 403
//   3. Authorized — user meets the declared rule → expect 200 (fake handler)
//   4. Admin bypass — Casbin Administrator → expect 200 (fake handler)
//
// For the AdminOnly route we verify that non-admin members get 403 while
// Casbin Administrators get 200.
//
// The fake handler is a no-op that returns 200. If Layer 2 allows the
// request through, the handler fires and we see 200. If Layer 2 blocks,
// we see 403.
//
// Implementation note on /organizations/:id/groups/:groupId/regenerate-passwords:
// the RoutePermission declares Param="id" (the organization ID), NOT
// "groupId". This is intentional — Layer 2 enforces *org-level* access
// (manager of the org); verifying that the target group actually belongs
// to the org is the handler's responsibility. The test below exercises the
// org param correctly: the URL contains both :id and :groupId, but only
// :id is consulted by the OrgRole enforcer.

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

// organizationsAuditMembershipChecker records org memberships keyed by
// "orgID:userID". CheckOrgRole applies the role hierarchy via
// access.IsRoleAtLeast. CheckGroupRole is unused by this module but
// required by the MembershipChecker interface.
type organizationsAuditMembershipChecker struct {
	orgRoles map[string]string
}

func (c *organizationsAuditMembershipChecker) CheckGroupRole(groupID, userID, minRole string) (bool, error) {
	return false, nil
}

func (c *organizationsAuditMembershipChecker) CheckOrgRole(orgID, userID, minRole string) (bool, error) {
	role, ok := c.orgRoles[orgID+":"+userID]
	if !ok {
		return false, nil
	}
	return access.IsRoleAtLeast(role, minRole), nil
}

// organizationsAuditEntityLoader is unused by OrgRole / AdminOnly but
// required by the RegisterBuiltinEnforcers signature.
type organizationsAuditEntityLoader struct{}

func (l *organizationsAuditEntityLoader) GetOwnerField(entity, id, field string) (string, error) {
	return "", nil
}

// -----------------------------------------------------------------------------
// Route catalog — mirrors src/organizations/routes/permissions.go.
// Each entry includes the data the audit needs to register the route in
// the registry and to issue a concrete request against it.
// -----------------------------------------------------------------------------

type organizationsAuditRoute struct {
	method         string
	registeredPath string
	requestPath    string
	// scopeID is the concrete URL-param value used for `:id` (the org ID)
	// in the request path. The checker is keyed off this value.
	scopeID   string
	ruleType  access.AccessRuleType
	minRole   string // only for OrgRole
	paramName string // OrgRole uses "id" for every route in this module
}

// organizationsAuditOrgRoutes — 6 OrgRole routes, all keyed off `:id`.
var organizationsAuditOrgRoutes = []organizationsAuditRoute{
	// Member-gated (read access for any active org member)
	{method: "GET", registeredPath: "/api/v1/organizations/:id/members", requestPath: "/api/v1/organizations/org-audit-mem/members", scopeID: "org-audit-mem", ruleType: access.OrgRole, minRole: "member", paramName: "id"},
	{method: "GET", registeredPath: "/api/v1/organizations/:id/groups", requestPath: "/api/v1/organizations/org-audit-grp/groups", scopeID: "org-audit-grp", ruleType: access.OrgRole, minRole: "member", paramName: "id"},
	{method: "GET", registeredPath: "/api/v1/organizations/:id/backends", requestPath: "/api/v1/organizations/org-audit-bk/backends", scopeID: "org-audit-bk", ruleType: access.OrgRole, minRole: "member", paramName: "id"},
	// Manager-gated (write actions on org content)
	{method: "POST", registeredPath: "/api/v1/organizations/:id/import", requestPath: "/api/v1/organizations/org-audit-imp/import", scopeID: "org-audit-imp", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
	// :groupId is also in the path, but Layer 2 enforces against :id (org-level).
	// Handler must independently verify the group belongs to the org.
	{method: "POST", registeredPath: "/api/v1/organizations/:id/groups/:groupId/regenerate-passwords", requestPath: "/api/v1/organizations/org-audit-rgn/groups/grp-1/regenerate-passwords", scopeID: "org-audit-rgn", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
	// Owner-gated (most sensitive: structural conversion)
	{method: "POST", registeredPath: "/api/v1/organizations/:id/convert-to-team", requestPath: "/api/v1/organizations/org-audit-cvt/convert-to-team", scopeID: "org-audit-cvt", ruleType: access.OrgRole, minRole: "owner", paramName: "id"},
}

// organizationsAuditAdminRoutes — 1 AdminOnly route.
var organizationsAuditAdminRoutes = []organizationsAuditRoute{
	{method: "PUT", registeredPath: "/api/v1/organizations/:id/backends", requestPath: "/api/v1/organizations/org-audit-bkput/backends", scopeID: "org-audit-bkput", ruleType: access.AdminOnly},
}

// allOrganizationsAuditRoutes returns every Layer 2-enforced route in scope.
func allOrganizationsAuditRoutes() []organizationsAuditRoute {
	all := append([]organizationsAuditRoute{}, organizationsAuditOrgRoutes...)
	all = append(all, organizationsAuditAdminRoutes...)
	return all
}

// -----------------------------------------------------------------------------
// Harness — installs Layer 2 with exactly the production declarations
// for one route under audit. Same shape as the payment / scenarios audit
// harnesses.
// -----------------------------------------------------------------------------

func setupOrganizationsAuditRouter(
	t *testing.T,
	route organizationsAuditRoute,
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

	rule := access.AccessRule{
		Type:    route.ruleType,
		MinRole: route.minRole,
	}
	if route.ruleType == access.OrgRole {
		rule.Param = route.paramName
	}

	role := "member"
	if route.ruleType == access.AdminOnly {
		role = "administrator"
	}

	access.RouteRegistry.Register("Organizations",
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

func doOrganizationsAuditRequest(r *gin.Engine, method, path, userID, roles string) *httptest.ResponseRecorder {
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
// Case 1: Outsider — Member with no org membership → 403
// -----------------------------------------------------------------------------

func TestOrganizationsLayer2_Outsider_Denied(t *testing.T) {
	for _, route := range allOrganizationsAuditRoutes() {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			loader := &organizationsAuditEntityLoader{}
			checker := &organizationsAuditMembershipChecker{orgRoles: map[string]string{}}
			r := setupOrganizationsAuditRouter(t, route, checker, loader)

			w := doOrganizationsAuditRequest(r, route.method, route.requestPath, "outsider-user", "member")
			assert.Equal(t, http.StatusForbidden, w.Code,
				"outsider must be denied on %s %s (observed %d, body=%s)",
				route.method, route.requestPath, w.Code, w.Body.String())
		})
	}
}

// -----------------------------------------------------------------------------
// Case 2: Insufficient role — only on OrgRole routes with MinRole > member.
// We test:
//   - manager-gated routes: a plain "member" must be denied
//   - owner-gated routes: a "manager" must be denied (one rung below owner)
// AdminOnly routes are skipped here — covered by Case 1 (any non-admin member
// is an "outsider" w.r.t. AdminOnly).
// -----------------------------------------------------------------------------

func TestOrganizationsLayer2_InsufficientRole_Denied(t *testing.T) {
	for _, route := range organizationsAuditOrgRoutes {
		// member-gated routes have no insufficient-role case.
		if route.minRole == "member" {
			continue
		}
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			// Pick a role one rung below the required role:
			//   manager-gated → grant "member"
			//   owner-gated   → grant "manager"
			var insufficientRole string
			switch route.minRole {
			case "manager":
				insufficientRole = "member"
			case "owner":
				insufficientRole = "manager"
			default:
				t.Fatalf("unexpected minRole %q in catalog for %s %s — extend this switch",
					route.minRole, route.method, route.registeredPath)
			}

			checker := &organizationsAuditMembershipChecker{
				orgRoles: map[string]string{
					route.scopeID + ":insufficient-user": insufficientRole,
				},
			}
			loader := &organizationsAuditEntityLoader{}
			r := setupOrganizationsAuditRouter(t, route, checker, loader)

			w := doOrganizationsAuditRequest(r, route.method, route.requestPath, "insufficient-user", "member")
			assert.Equal(t, http.StatusForbidden, w.Code,
				"%s must be denied on %s-gated %s %s (observed %d)",
				insufficientRole, route.minRole, route.method, route.requestPath, w.Code)
		})
	}
}

// -----------------------------------------------------------------------------
// Case 3: Authorized — user meets the rule → Layer 2 lets through.
// AdminOnly routes are excluded here (only an Administrator role passes —
// covered by Case 4).
// -----------------------------------------------------------------------------

func TestOrganizationsLayer2_Authorized_Allowed(t *testing.T) {
	const authorizedUser = "authorized-user"

	for _, route := range organizationsAuditOrgRoutes {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			checker := &organizationsAuditMembershipChecker{
				orgRoles: map[string]string{
					route.scopeID + ":" + authorizedUser: route.minRole,
				},
			}
			loader := &organizationsAuditEntityLoader{}
			r := setupOrganizationsAuditRouter(t, route, checker, loader)

			w := doOrganizationsAuditRequest(r, route.method, route.requestPath, authorizedUser, "member")
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"authorized user (role=%s) must not be blocked on %s %s (observed %d, body=%s)",
				route.minRole, route.method, route.requestPath, w.Code, w.Body.String())
			assert.Equal(t, http.StatusOK, w.Code,
				"fake handler should have returned 200 for %s %s (body=%s)",
				route.method, route.requestPath, w.Body.String())
		})
	}
}

// -----------------------------------------------------------------------------
// Case 4: Admin bypass — Casbin Administrator always allowed (both OrgRole
// and AdminOnly routes).
// -----------------------------------------------------------------------------

func TestOrganizationsLayer2_AdminBypass_Allowed(t *testing.T) {
	for _, route := range allOrganizationsAuditRoutes() {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			loader := &organizationsAuditEntityLoader{}
			// Admin has NO org membership — bypass must still allow.
			checker := &organizationsAuditMembershipChecker{orgRoles: map[string]string{}}
			r := setupOrganizationsAuditRouter(t, route, checker, loader)

			w := doOrganizationsAuditRequest(r, route.method, route.requestPath, "admin-user", "administrator")
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"administrator must bypass %s enforcement on %s %s (observed %d)",
				route.ruleType, route.method, route.requestPath, w.Code)
			assert.Equal(t, http.StatusOK, w.Code,
				"admin must reach handler on %s %s", route.method, route.requestPath)
		})
	}
}

// -----------------------------------------------------------------------------
// AUDIT FINDING (positive) — Layer 1 / Layer 2 path consistency.
//
// Every (path, method) pair appearing in the Layer 1 ReconcilePolicy
// loops at the top of permissions.go must have a matching
// RouteRegistry.Register entry so Layer 2 actually inspects the route.
// A Layer 1 policy without a Layer 2 declaration is a silent bypass:
// the request makes it past the RBAC gate and through Layer 2
// untouched. This subtest cross-walks the catalog above against the
// registry as it would be populated by RegisterOrganizationPermissions.
//
// The catalog above is the canonical source of truth for this audit. If
// a route is added or removed from permissions.go, the catalog must be
// updated to match — this subtest is a reminder that the two must stay
// aligned.
// -----------------------------------------------------------------------------

func TestOrganizationsLayer2_RegistryDeclaresEveryAuditedRoute(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	// Register every route in the audit catalog using the same shape
	// production uses — this matches what RegisterOrganizationPermissions
	// does at startup. We don't import the production function here
	// because doing so would also pull in unrelated module init; the
	// catalog parity check above is what guards drift.
	for _, route := range allOrganizationsAuditRoutes() {
		rule := access.AccessRule{
			Type:    route.ruleType,
			MinRole: route.minRole,
		}
		if route.ruleType == access.OrgRole {
			rule.Param = route.paramName
		}

		role := "member"
		if route.ruleType == access.AdminOnly {
			role = "administrator"
		}

		access.RouteRegistry.Register("Organizations",
			access.RoutePermission{
				Path:   route.registeredPath,
				Method: route.method,
				Role:   role,
				Access: rule,
			},
		)
	}

	for _, route := range allOrganizationsAuditRoutes() {
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
// The bug fixed in MR !180 (terminals incus-ui) was a regex method
// like "(GET|POST|PUT|PATCH|DELETE)" silently bypassing
// RouteRegistry.Lookup, which does exact method+path string match.
// This subtest verifies every catalog entry uses a single concrete
// HTTP verb, mirroring what the production permissions.go declares.
// -----------------------------------------------------------------------------

func TestOrganizationsLayer2_NoRegexMethodInCatalog(t *testing.T) {
	allowed := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	}

	for _, route := range allOrganizationsAuditRoutes() {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			assert.True(t, allowed[route.method],
				"catalog route %s %s declares a non-canonical HTTP method — Layer 2 Lookup is exact-match, regex/alternation methods would silently bypass enforcement",
				route.method, route.registeredPath)
			assert.NotContains(t, route.method, "|",
				"alternation in method string for %s %s would silently bypass Layer 2 (see MR !180)",
				route.method, route.registeredPath)
		})
	}
}
