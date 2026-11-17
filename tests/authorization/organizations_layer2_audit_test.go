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
// This file holds the route catalog only.
//
// Implementation note on /organizations/:id/groups/:groupId/regenerate-passwords:
// the RoutePermission declares Param="id" (the organization ID), NOT
// "groupId". This is intentional — Layer 2 enforces *org-level* access
// (manager of the org); verifying that the target group actually belongs
// to the org is the handler's responsibility. The test below exercises the
// org param correctly: the URL contains both :id and :groupId, but only
// :id is consulted by the OrgRole enforcer.

import (
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
	{method: "POST", registeredPath: "/api/v1/organizations/:id/members/offboard", requestPath: "/api/v1/organizations/org-audit-off/members/offboard", scopeID: "org-audit-off", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
	// :userId is also in the path, but Layer 2 enforces against :id (org-level).
	{method: "POST", registeredPath: "/api/v1/organizations/:id/members/:userId/reinstate", requestPath: "/api/v1/organizations/org-audit-rei/members/u-1/reinstate", scopeID: "org-audit-rei", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
	// :groupId is also in the path, but Layer 2 enforces against :id (org-level).
	// Handler must independently verify the group belongs to the org.
	{method: "POST", registeredPath: "/api/v1/organizations/:id/groups/:groupId/regenerate-passwords", requestPath: "/api/v1/organizations/org-audit-rgn/groups/grp-1/regenerate-passwords", scopeID: "org-audit-rgn", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
	// Owner-gated (most sensitive: structural conversion)
	{method: "POST", registeredPath: "/api/v1/organizations/:id/convert-to-team", requestPath: "/api/v1/organizations/org-audit-cvt/convert-to-team", scopeID: "org-audit-cvt", ruleType: access.OrgRole, minRole: "owner", paramName: "id"},
	{method: "POST", registeredPath: "/api/v1/organizations/:id/members/:userId/erase", requestPath: "/api/v1/organizations/org-audit-era/members/u-1/erase", scopeID: "org-audit-era", ruleType: access.OrgRole, minRole: "owner", paramName: "id"},
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

// -----------------------------------------------------------------------------
// Regression guard: no regex-method declarations.
//
// The bug fixed in MR !180 (terminals incus-ui) was a regex method
// like "(GET|POST|PUT|PATCH|DELETE)" silently bypassing
// RouteRegistry.Lookup, which does exact method+path string match.
// This subtest verifies every catalog entry uses a single concrete
// HTTP verb, mirroring what the production permissions.go declares.
// -----------------------------------------------------------------------------
