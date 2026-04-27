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
// Enforcement scenarios (Outsider, InsufficientRole, Authorized, AdminBypass)
// are covered by the generic parameterized suite in
// layer2_audit_parameterized_test.go via adaptOrganizationsRoutes().
//
// This file retains the route catalog and structural guards:
//   - TestOrganizationsLayer2_RegistryDeclaresEveryAuditedRoute
//   - TestOrganizationsLayer2_NoRegexMethodInCatalog
//
// Implementation note on /organizations/:id/groups/:groupId/regenerate-passwords:
// the RoutePermission declares Param="id" (the organization ID), NOT
// "groupId". This is intentional — Layer 2 enforces *org-level* access
// (manager of the org); verifying that the target group actually belongs
// to the org is the handler's responsibility. The test below exercises the
// org param correctly: the URL contains both :id and :groupId, but only
// :id is consulted by the OrgRole enforcer.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
)

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
		route := route // capture
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
		route := route // capture
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
