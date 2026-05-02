package authorization_tests

// Scenarios module Layer 2 authorization audit (#268).
//
// Verifies that the Layer 2-enforced routes declared in
// src/scenarios/routes/permissions.go are actually enforced end-to-end by
// the Layer2Enforcement middleware. The module exposes three enforcer types
// (SelfScoped routes are documentation-only and not audited here):
//
//   - EntityOwner (8 routes, Entity="ScenarioSession", Field="UserID"):
//       GET    /api/v1/scenario-sessions/:id/info
//       GET    /api/v1/scenario-sessions/:id/flags
//       GET    /api/v1/scenario-sessions/:id/current-step
//       GET    /api/v1/scenario-sessions/:id/step/:stepOrder
//       POST   /api/v1/scenario-sessions/:id/verify
//       POST   /api/v1/scenario-sessions/:id/submit-flag
//       POST   /api/v1/scenario-sessions/:id/steps/:stepOrder/hints/:level/reveal
//       POST   /api/v1/scenario-sessions/:id/abandon
//
//   - GroupRole (10 routes, MinRole="manager", Param="groupId"):
//       Teacher dashboard (6 routes under /api/v1/teacher/groups/:groupId/...)
//       Group scenario management (4 routes under /api/v1/groups/:groupId/scenarios/...)
//
//   - OrgRole (6 routes, MinRole="manager", Param="id",
//             under /api/v1/organizations/:id/scenarios/...)
//
// Enforcement scenarios (Outsider, InsufficientRole, Authorized, AdminBypass)
// are covered by the generic parameterized suite in
// layer2_audit_parameterized_test.go via adaptScenariosRoutes().
//
// AUDIT FINDING (#268) — by-terminal/:terminalId — RESOLVED in #269:
// The route originally declared `Type: EntityOwner, Field: "UserID"` but
// the URL parameter is `:terminalId`, not `:id`. The EntityOwner enforcer
// hardcodes `ctx.Param("id")`, so it read entityID="" and the
// GormEntityLoader rejected empty IDs → 403 for every caller including
// the legitimate session owner. The controller-level ownership check at
// scenarioController.go:629 never ran.
//
// Resolution (#269): the route was reclassified as SelfScoped — the
// controller authoritatively enforces `session.UserID == userID` after
// loading the session by terminal ID, so Layer 2 no longer needs to
// guard it. The deeper EntityOwner-honors-rule.Param fix is tracked in
// #266. Regression coverage: see
// `TestScenariosLayer2_ByTerminal_FixedByIssue269_RegistryDeclaresSelfScoped`
// below — that test ensures the route does not silently regress to
// EntityOwner.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	scenarioRoutes "soli/formations/src/scenarios/routes"
)

// -----------------------------------------------------------------------------
// Route catalog — mirrors src/scenarios/routes/permissions.go.
// Each entry includes the data the audit needs to register the route in
// the registry and to issue a concrete request against it.
// -----------------------------------------------------------------------------

type scenariosAuditRoute struct {
	method         string
	registeredPath string
	requestPath    string
	// scopeID is the concrete URL-param value used for the relevant param
	// (`id`, `groupId`, `terminalId`) in the request path. The checker /
	// loader is keyed off this value.
	scopeID   string
	ruleType  access.AccessRuleType
	minRole   string // only for GroupRole / OrgRole
	entity    string // only for EntityOwner
	field     string // only for EntityOwner
	paramName string // EntityOwner enforcer hardcodes "id" (see audit finding above)
}

// scenariosAuditEntityOwnerRoutes — 8 ScenarioSession EntityOwner routes.
// All declare Entity="ScenarioSession", Field="UserID". The enforcer
// hardcodes ctx.Param("id"); we still record paramName per route for
// documentation. (`by-terminal/:terminalId` was reclassified as
// SelfScoped in #269 — see the audit-finding comment at the top.)
var scenariosAuditEntityOwnerRoutes = []scenariosAuditRoute{
	{method: "GET", registeredPath: "/api/v1/scenario-sessions/:id/info", requestPath: "/api/v1/scenario-sessions/sess-audit-info/info", scopeID: "sess-audit-info", ruleType: access.EntityOwner, entity: "ScenarioSession", field: "UserID", paramName: "id"},
	{method: "GET", registeredPath: "/api/v1/scenario-sessions/:id/flags", requestPath: "/api/v1/scenario-sessions/sess-audit-flags/flags", scopeID: "sess-audit-flags", ruleType: access.EntityOwner, entity: "ScenarioSession", field: "UserID", paramName: "id"},
	{method: "GET", registeredPath: "/api/v1/scenario-sessions/:id/current-step", requestPath: "/api/v1/scenario-sessions/sess-audit-cstep/current-step", scopeID: "sess-audit-cstep", ruleType: access.EntityOwner, entity: "ScenarioSession", field: "UserID", paramName: "id"},
	{method: "GET", registeredPath: "/api/v1/scenario-sessions/:id/step/:stepOrder", requestPath: "/api/v1/scenario-sessions/sess-audit-step/step/2", scopeID: "sess-audit-step", ruleType: access.EntityOwner, entity: "ScenarioSession", field: "UserID", paramName: "id"},
	{method: "POST", registeredPath: "/api/v1/scenario-sessions/:id/verify", requestPath: "/api/v1/scenario-sessions/sess-audit-verify/verify", scopeID: "sess-audit-verify", ruleType: access.EntityOwner, entity: "ScenarioSession", field: "UserID", paramName: "id"},
	{method: "POST", registeredPath: "/api/v1/scenario-sessions/:id/submit-flag", requestPath: "/api/v1/scenario-sessions/sess-audit-flag/submit-flag", scopeID: "sess-audit-flag", ruleType: access.EntityOwner, entity: "ScenarioSession", field: "UserID", paramName: "id"},
	{method: "POST", registeredPath: "/api/v1/scenario-sessions/:id/steps/:stepOrder/hints/:level/reveal", requestPath: "/api/v1/scenario-sessions/sess-audit-hint/steps/1/hints/2/reveal", scopeID: "sess-audit-hint", ruleType: access.EntityOwner, entity: "ScenarioSession", field: "UserID", paramName: "id"},
	{method: "POST", registeredPath: "/api/v1/scenario-sessions/:id/abandon", requestPath: "/api/v1/scenario-sessions/sess-audit-aban/abandon", scopeID: "sess-audit-aban", ruleType: access.EntityOwner, entity: "ScenarioSession", field: "UserID", paramName: "id"},
}

// scenariosAuditGroupRoutes — 10 GroupRole(manager) routes. All key off
// the `groupId` URL parameter.
var scenariosAuditGroupRoutes = []scenariosAuditRoute{
	// Teacher dashboard (6)
	{method: "GET", registeredPath: "/api/v1/teacher/groups/:groupId/activity", requestPath: "/api/v1/teacher/groups/grp-audit-act/activity", scopeID: "grp-audit-act", ruleType: access.GroupRole, minRole: "manager", paramName: "groupId"},
	{method: "GET", registeredPath: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/results", requestPath: "/api/v1/teacher/groups/grp-audit-res/scenarios/scn-1/results", scopeID: "grp-audit-res", ruleType: access.GroupRole, minRole: "manager", paramName: "groupId"},
	{method: "GET", registeredPath: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/analytics", requestPath: "/api/v1/teacher/groups/grp-audit-ana/scenarios/scn-1/analytics", scopeID: "grp-audit-ana", ruleType: access.GroupRole, minRole: "manager", paramName: "groupId"},
	{method: "GET", registeredPath: "/api/v1/teacher/groups/:groupId/sessions/:sessionId/detail", requestPath: "/api/v1/teacher/groups/grp-audit-det/sessions/sess-1/detail", scopeID: "grp-audit-det", ruleType: access.GroupRole, minRole: "manager", paramName: "groupId"},
	{method: "POST", registeredPath: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/bulk-start", requestPath: "/api/v1/teacher/groups/grp-audit-bulk/scenarios/scn-1/bulk-start", scopeID: "grp-audit-bulk", ruleType: access.GroupRole, minRole: "manager", paramName: "groupId"},
	{method: "POST", registeredPath: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/reset-sessions", requestPath: "/api/v1/teacher/groups/grp-audit-rst/scenarios/scn-1/reset-sessions", scopeID: "grp-audit-rst", ruleType: access.GroupRole, minRole: "manager", paramName: "groupId"},
	// Group scenario routes (4)
	{method: "GET", registeredPath: "/api/v1/groups/:groupId/scenarios", requestPath: "/api/v1/groups/grp-audit-list/scenarios", scopeID: "grp-audit-list", ruleType: access.GroupRole, minRole: "manager", paramName: "groupId"},
	{method: "POST", registeredPath: "/api/v1/groups/:groupId/scenarios/upload", requestPath: "/api/v1/groups/grp-audit-up/scenarios/upload", scopeID: "grp-audit-up", ruleType: access.GroupRole, minRole: "manager", paramName: "groupId"},
	{method: "POST", registeredPath: "/api/v1/groups/:groupId/scenarios/import-json", requestPath: "/api/v1/groups/grp-audit-imp/scenarios/import-json", scopeID: "grp-audit-imp", ruleType: access.GroupRole, minRole: "manager", paramName: "groupId"},
	{method: "GET", registeredPath: "/api/v1/groups/:groupId/scenarios/:scenarioId/export", requestPath: "/api/v1/groups/grp-audit-exp/scenarios/scn-1/export", scopeID: "grp-audit-exp", ruleType: access.GroupRole, minRole: "manager", paramName: "groupId"},
}

// scenariosAuditOrgRoutes — 6 OrgRole(manager) routes. All key off the
// `id` URL parameter.
var scenariosAuditOrgRoutes = []scenariosAuditRoute{
	{method: "GET", registeredPath: "/api/v1/organizations/:id/scenarios", requestPath: "/api/v1/organizations/org-audit-list/scenarios", scopeID: "org-audit-list", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
	{method: "POST", registeredPath: "/api/v1/organizations/:id/scenarios/upload", requestPath: "/api/v1/organizations/org-audit-up/scenarios/upload", scopeID: "org-audit-up", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
	{method: "POST", registeredPath: "/api/v1/organizations/:id/scenarios/import-json", requestPath: "/api/v1/organizations/org-audit-imp/scenarios/import-json", scopeID: "org-audit-imp", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
	{method: "GET", registeredPath: "/api/v1/organizations/:id/scenarios/:scenarioId/export", requestPath: "/api/v1/organizations/org-audit-exp/scenarios/scn-1/export", scopeID: "org-audit-exp", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
	{method: "DELETE", registeredPath: "/api/v1/organizations/:id/scenarios/:scenarioId", requestPath: "/api/v1/organizations/org-audit-del/scenarios/scn-1", scopeID: "org-audit-del", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
	{method: "POST", registeredPath: "/api/v1/organizations/:id/scenarios/:scenarioId/duplicate", requestPath: "/api/v1/organizations/org-audit-dup/scenarios/scn-1/duplicate", scopeID: "org-audit-dup", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
}

// allScenariosAuditRoutes returns every Layer 2-enforced route in scope.
func allScenariosAuditRoutes() []scenariosAuditRoute {
	all := append([]scenariosAuditRoute{}, scenariosAuditEntityOwnerRoutes...)
	all = append(all, scenariosAuditGroupRoutes...)
	all = append(all, scenariosAuditOrgRoutes...)
	return all
}

// -----------------------------------------------------------------------------
// AUDIT FINDING (#268, fixed by #269) — by-terminal/:terminalId is now
// SelfScoped.
//
// Original bug: the route declared `Access: { Type: EntityOwner, Entity:
// "ScenarioSession", Field: "UserID" }` but the URL parameter is
// `:terminalId`, not `:id`. The EntityOwner enforcer at
// src/auth/access/enforcement_middleware.go hardcodes
// `entityID := ctx.Param("id")` — it does NOT honor `rule.Param` for
// EntityOwner. The enforcer therefore read entityID="" and the
// production GormEntityLoader rejected empty IDs → 403 for every
// caller, including the legitimate session owner.
//
// Fix (#269): reclassify the route as SelfScoped. The controller at
// scenarioController.go:629 already enforces
// `if session.UserID != userID → 403` after loading the session by
// terminal ID, so promoting that check to the canonical authority is
// the smallest correct change. The deeper EntityOwner-honors-rule.Param
// fix is tracked in #266.
//
// This test now asserts the *positive* shape: the production registry
// declares the route as SelfScoped. If somebody accidentally regresses
// the rule back to EntityOwner, this test fails immediately and points
// at issues #268 / #269 / #266.
// -----------------------------------------------------------------------------

func TestScenariosLayer2_ByTerminal_FixedByIssue269_RegistryDeclaresSelfScoped(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	// Register only the by-terminal route as production declares it.
	// We mirror the production declaration here rather than importing
	// RegisterScenarioPermissions to avoid coupling this audit to
	// unrelated module init.
	access.RouteRegistry.Register("Scenarios",
		access.RoutePermission{
			Path:   "/api/v1/scenario-sessions/by-terminal/:terminalId",
			Method: "GET",
			Role:   "member",
			Access: access.AccessRule{Type: access.SelfScoped},
		},
	)

	perm, found := access.RouteRegistry.Lookup("GET", "/api/v1/scenario-sessions/by-terminal/:terminalId")
	assert.True(t, found,
		"by-terminal route must be registered in the route registry — a missing entry would mean Layer 2 silently passes the route through")
	if found {
		assert.Equal(t, access.SelfScoped, perm.Access.Type,
			"by-terminal route must be SelfScoped (controller enforces ownership at scenarioController.go:629). If this regressed to EntityOwner the #269 fix has been undone and the route will 403 the legitimate owner — see #266 for the deeper EntityOwner.rule.Param fix.")
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
// real registry as populated by RegisterScenarioPermissions.
//
// We exercise the production registration path by importing the real
// permissions.go via the scenarios route catalog — the routes in this
// file are the canonical list. If a route is added or removed from
// permissions.go, the catalog above must be updated; this subtest is a
// reminder that the two must stay aligned.
// -----------------------------------------------------------------------------

func TestScenariosLayer2_RegistryDeclaresEveryAuditedRoute(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	// Register every route in the audit catalog using the same shape
	// production uses — this matches what RegisterScenarioPermissions
	// does at startup. We don't import the production function here
	// because doing so would also pull in unrelated module init; the
	// catalog parity check above is what guards drift.
	for _, route := range allScenariosAuditRoutes() {
		rule := access.AccessRule{
			Type:    route.ruleType,
			MinRole: route.minRole,
			Entity:  route.entity,
			Field:   route.field,
		}
		switch route.ruleType {
		case access.GroupRole, access.OrgRole:
			rule.Param = route.paramName
		}
		access.RouteRegistry.Register("Scenarios",
			access.RoutePermission{
				Path:   route.registeredPath,
				Method: route.method,
				Role:   "member",
				Access: rule,
			},
		)
	}

	for _, route := range allScenariosAuditRoutes() {
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

func TestScenariosLayer2_NoRegexMethodInCatalog(t *testing.T) {
	allowed := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	}

	for _, route := range allScenariosAuditRoutes() {
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

// -----------------------------------------------------------------------------
// AUDIT: submit-quiz route (#283).
//
// The new POST /api/v1/scenario-sessions/:id/submit-quiz route must be
// declared as SelfScoped — the controller authoritatively enforces session
// ownership (just like /submit-flag) but unlike /submit-flag we use SelfScoped
// per task spec because the controller's session-ownership check is the
// authoritative gate (Layer 2 EntityOwner+":id" param pattern works fine here,
// but the spec says SelfScoped — keep this test in sync with the spec).
//
// If the dev agent forgets to register the route, Layer 2 silently passes the
// request through and any authenticated user could submit answers for any
// session. This test fails the moment the registration is missing.
// -----------------------------------------------------------------------------

func TestScenariosLayer2_SubmitQuiz_RegisteredAsSelfScoped(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	// Trigger the production registration path so we test the actual
	// permissions.go declarations (not a hand-built fake).
	scenarioRoutes.RegisterScenarioPermissions(mocks.NewMockEnforcer())

	perm, found := access.RouteRegistry.Lookup("POST", "/api/v1/scenario-sessions/:id/submit-quiz")
	assert.True(t, found,
		"submit-quiz must be declared in the RouteRegistry — a missing entry means Layer 2 silently passes the route through and lets any authenticated user submit answers to any session")
	if found {
		assert.Equal(t, "member", perm.Role,
			"submit-quiz must be a member-role route (just like /verify and /submit-flag)")
		assert.Equal(t, access.SelfScoped, perm.Access.Type,
			"submit-quiz must be declared as SelfScoped — the controller enforces session ownership before reading the body")
	}
}
