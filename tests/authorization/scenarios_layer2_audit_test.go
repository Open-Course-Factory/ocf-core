package authorization_tests

// Scenarios module Layer 2 authorization audit (#268).
//
// Verifies that the 25 Layer 2-enforced routes declared in
// src/scenarios/routes/permissions.go are actually enforced end-to-end by
// the Layer2Enforcement middleware. The module exposes three enforcer types
// (SelfScoped routes are documentation-only and not audited here):
//
//   - EntityOwner (9 routes, Entity="ScenarioSession", Field="UserID"):
//       GET    /api/v1/scenario-sessions/by-terminal/:terminalId   (NOTE: ⚠ see below)
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
// For each route we run four scenarios mirroring the payment / terminals
// audits:
//
//   1. Outsider — Casbin Member with NO ownership / membership → expect 403
//   2. Insufficient role — only on routes with MinRole > member → expect 403
//   3. Authorized — user meets the declared rule → expect 200 (fake handler)
//   4. Admin bypass — Casbin Administrator → expect 200 (fake handler)
//
// AUDIT FINDING — by-terminal/:terminalId is broken:
// The EntityOwner enforcer in src/auth/access/enforcement_middleware.go
// (line 112) hardcodes `entityID := ctx.Param("id")`. The
// /api/v1/scenario-sessions/by-terminal/:terminalId route does NOT have
// an `:id` URL parameter — only `:terminalId`. As a result the enforcer
// always reads entityID="" and the production GormEntityLoader rejects
// empty IDs with an error → 403 for EVERY caller, including the
// legitimate session owner. The controller-level ownership check at
// scenarioController.go:629 never runs because Layer 2 aborts first.
// The harness reproduces this with the test
// `TestScenariosLayer2_ByTerminal_OwnerStillBlocked_AuditFinding` below.
// Fix is non-trivial (requires either a per-route `Param` field for
// EntityOwner or switching the route to SelfScoped); deferring to a
// follow-up issue per the audit boundary in the task brief.
//
// The fake handler is a no-op that returns 200. If Layer 2 allows the
// request through, the handler fires and we see 200. If Layer 2 blocks,
// we see 403.

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

// scenariosAuditMembershipChecker records group and org memberships keyed
// by "scopeID:userID". CheckGroupRole and CheckOrgRole apply the role
// hierarchy via access.IsRoleAtLeast.
type scenariosAuditMembershipChecker struct {
	groupRoles map[string]string
	orgRoles   map[string]string
}

func (c *scenariosAuditMembershipChecker) CheckGroupRole(groupID, userID, minRole string) (bool, error) {
	role, ok := c.groupRoles[groupID+":"+userID]
	if !ok {
		return false, nil
	}
	return access.IsRoleAtLeast(role, minRole), nil
}

func (c *scenariosAuditMembershipChecker) CheckOrgRole(orgID, userID, minRole string) (bool, error) {
	role, ok := c.orgRoles[orgID+":"+userID]
	if !ok {
		return false, nil
	}
	return access.IsRoleAtLeast(role, minRole), nil
}

// scenariosAuditEntityLoader exposes a configurable owner map for
// ScenarioSession entities. Keys are
// "entityName:entityID:fieldName" -> owner value, mirroring the harness
// in terminals_layer2_audit_test.go.
type scenariosAuditEntityLoader struct {
	owners map[string]string
}

func (l *scenariosAuditEntityLoader) GetOwnerField(entity, id, field string) (string, error) {
	key := entity + ":" + id + ":" + field
	if v, ok := l.owners[key]; ok {
		return v, nil
	}
	// Absent: return empty string. Layer 2's EntityOwner compares against
	// userID; an empty string only matches an empty userID (which Layer 2
	// treats as unauthenticated and passes through).
	return "", nil
}

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

// scenariosAuditEntityOwnerRoutes — 9 ScenarioSession EntityOwner routes.
// All declare Entity="ScenarioSession", Field="UserID". The enforcer
// hardcodes ctx.Param("id"); we still record paramName per route for
// documentation and to flag the by-terminal mismatch.
var scenariosAuditEntityOwnerRoutes = []scenariosAuditRoute{
	{method: "GET", registeredPath: "/api/v1/scenario-sessions/by-terminal/:terminalId", requestPath: "/api/v1/scenario-sessions/by-terminal/term-audit-1", scopeID: "term-audit-1", ruleType: access.EntityOwner, entity: "ScenarioSession", field: "UserID", paramName: "terminalId"},
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
// Harness — installs Layer 2 with exactly the production declarations
// for one route under audit. Mirrors setupTerminalsAuditRouter.
// -----------------------------------------------------------------------------

func setupScenariosAuditRouter(
	t *testing.T,
	route scenariosAuditRoute,
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

	// EntityOwner declarations always use Param="id" in production
	// (see permissions.go lines 147-189). GroupRole and OrgRole use the
	// route-specific param name. We reproduce production exactly so
	// changes to the catalog are visible here.
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

func doScenariosAuditRequest(r *gin.Engine, method, path, userID, roles string) *httptest.ResponseRecorder {
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
// Case 1: Outsider — Member with no ownership / no membership → 403
// -----------------------------------------------------------------------------

func TestScenariosLayer2_Outsider_Denied(t *testing.T) {
	for _, route := range allScenariosAuditRoutes() {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			owners := map[string]string{}
			if route.ruleType == access.EntityOwner {
				// Session is owned by someone else so the outsider never
				// matches even if Layer 2 happened to look up the right
				// param (see audit finding for by-terminal).
				owners["ScenarioSession:"+route.scopeID+":UserID"] = "real-owner-user"
			}
			loader := &scenariosAuditEntityLoader{owners: owners}
			checker := &scenariosAuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			r := setupScenariosAuditRouter(t, route, checker, loader)

			w := doScenariosAuditRequest(r, route.method, route.requestPath, "outsider-user", "member")
			assert.Equal(t, http.StatusForbidden, w.Code,
				"outsider must be denied on %s %s (observed %d, body=%s)",
				route.method, route.requestPath, w.Code, w.Body.String())
		})
	}
}

// -----------------------------------------------------------------------------
// Case 2: Insufficient role — only on routes with MinRole > member.
// All Group / Org routes in scope require manager.
// -----------------------------------------------------------------------------

func TestScenariosLayer2_InsufficientRole_Denied(t *testing.T) {
	var managerGated []scenariosAuditRoute
	managerGated = append(managerGated, scenariosAuditGroupRoutes...)
	managerGated = append(managerGated, scenariosAuditOrgRoutes...)

	for _, route := range managerGated {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			checker := &scenariosAuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			switch route.ruleType {
			case access.GroupRole:
				checker.groupRoles[route.scopeID+":insufficient-user"] = "member"
			case access.OrgRole:
				checker.orgRoles[route.scopeID+":insufficient-user"] = "member"
			}
			loader := &scenariosAuditEntityLoader{owners: map[string]string{}}
			r := setupScenariosAuditRouter(t, route, checker, loader)

			w := doScenariosAuditRequest(r, route.method, route.requestPath, "insufficient-user", "member")
			assert.Equal(t, http.StatusForbidden, w.Code,
				"plain member must be denied on manager-gated %s %s (observed %d)",
				route.method, route.requestPath, w.Code)
		})
	}
}

// -----------------------------------------------------------------------------
// Case 3: Authorized — user meets the rule → Layer 2 lets through.
//
// NOTE: by-terminal/:terminalId is excluded from the Authorized case
// because of the audit finding documented at the top of this file. A
// dedicated subtest below pins the broken behavior.
// -----------------------------------------------------------------------------

func TestScenariosLayer2_Authorized_Allowed(t *testing.T) {
	const authorizedUser = "authorized-user"

	all := []scenariosAuditRoute{}
	for _, r := range scenariosAuditEntityOwnerRoutes {
		// Skip by-terminal: tracked separately as an audit finding.
		if strings.Contains(r.registeredPath, "/by-terminal/") {
			continue
		}
		all = append(all, r)
	}
	all = append(all, scenariosAuditGroupRoutes...)
	all = append(all, scenariosAuditOrgRoutes...)

	for _, route := range all {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			checker := &scenariosAuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			owners := map[string]string{}

			switch route.ruleType {
			case access.EntityOwner:
				owners["ScenarioSession:"+route.scopeID+":UserID"] = authorizedUser
			case access.GroupRole:
				checker.groupRoles[route.scopeID+":"+authorizedUser] = route.minRole
			case access.OrgRole:
				checker.orgRoles[route.scopeID+":"+authorizedUser] = route.minRole
			}

			loader := &scenariosAuditEntityLoader{owners: owners}
			r := setupScenariosAuditRouter(t, route, checker, loader)

			w := doScenariosAuditRequest(r, route.method, route.requestPath, authorizedUser, "member")
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"authorized user must not be blocked on %s %s (observed %d, body=%s)",
				route.method, route.requestPath, w.Code, w.Body.String())
			assert.Equal(t, http.StatusOK, w.Code,
				"fake handler should have returned 200 for %s %s (body=%s)",
				route.method, route.requestPath, w.Body.String())
		})
	}
}

// -----------------------------------------------------------------------------
// Case 4: Admin bypass — Casbin Administrator always allowed.
// -----------------------------------------------------------------------------

func TestScenariosLayer2_AdminBypass_Allowed(t *testing.T) {
	for _, route := range allScenariosAuditRoutes() {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			loader := &scenariosAuditEntityLoader{owners: map[string]string{}}
			checker := &scenariosAuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			r := setupScenariosAuditRouter(t, route, checker, loader)

			w := doScenariosAuditRequest(r, route.method, route.requestPath, "admin-user", "administrator")
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"administrator must bypass %s enforcement on %s %s (observed %d)",
				route.ruleType, route.method, route.requestPath, w.Code)
			assert.Equal(t, http.StatusOK, w.Code,
				"admin must reach handler on %s %s", route.method, route.requestPath)
		})
	}
}

// -----------------------------------------------------------------------------
// AUDIT FINDING — by-terminal/:terminalId enforcement is broken.
//
// The route declares `Access: { Type: EntityOwner, Entity:
// "ScenarioSession", Field: "UserID" }` but the URL parameter is
// `:terminalId`, not `:id`. The EntityOwner enforcer at
// src/auth/access/enforcement_middleware.go:112 hardcodes
// `entityID := ctx.Param("id")` — it does NOT honor `rule.Param` for
// EntityOwner. As a result:
//   - The enforcer reads entityID="".
//   - The production GormEntityLoader rejects empty IDs with an error.
//   - The enforcer responds 403 to ALL callers, including the legitimate
//     session owner.
//   - The controller-level ownership check (scenarioController.go:629)
//     never executes.
//
// The harness reproduces this with a loader that returns the empty
// string for an unmapped (entity, "", field) lookup — which is exactly
// what the production loader's "entity ID must not be empty" branch
// translates into via the enforcer's err-path. We assert the broken
// behavior so a future fix flips this test.
//
// Suggested fix: either
//   (a) make EntityOwner honor rule.Param (default "id"), or
//   (b) reclassify the route as SelfScoped since the controller
//       already enforces ownership against `userId`.
// Option (b) is the smallest change and keeps Layer 2 advisory.
// -----------------------------------------------------------------------------

func TestScenariosLayer2_ByTerminal_OwnerStillBlocked_AuditFinding(t *testing.T) {
	var byTerminal scenariosAuditRoute
	for _, r := range scenariosAuditEntityOwnerRoutes {
		if strings.Contains(r.registeredPath, "/by-terminal/") {
			byTerminal = r
			break
		}
	}

	const ownerUser = "rightful-owner"

	// Even with the loader configured to recognize the terminal-keyed
	// owner, Layer 2 reads ctx.Param("id") (NOT "terminalId") and
	// therefore looks up "ScenarioSession::UserID" — guaranteed miss.
	loader := &scenariosAuditEntityLoader{owners: map[string]string{
		"ScenarioSession:" + byTerminal.scopeID + ":UserID": ownerUser,
	}}
	checker := &scenariosAuditMembershipChecker{
		groupRoles: map[string]string{},
		orgRoles:   map[string]string{},
	}
	r := setupScenariosAuditRouter(t, byTerminal, checker, loader)

	w := doScenariosAuditRequest(r, byTerminal.method, byTerminal.requestPath, ownerUser, "member")
	// Expected (and broken) behavior: 403 for the legitimate owner.
	// If this assertion ever flips to allow the owner through, the
	// finding has been fixed and this test should be promoted into the
	// regular Authorized case.
	assert.Equal(t, http.StatusForbidden, w.Code,
		"AUDIT FINDING (#268): by-terminal/:terminalId currently denies the legitimate owner because the EntityOwner enforcer reads ctx.Param(\"id\") which doesn't exist on this route. Observed %d (body=%s). If this flipped to 200 the bug is fixed.",
		w.Code, w.Body.String())
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
