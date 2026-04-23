package authorization_tests

// Terminals module Layer 2 authorization audit (#264).
//
// Verifies that the 12 Layer 2-reliant terminal routes declared in
// src/terminalTrainer/routes/permissions.go are actually enforced
// end-to-end by the Layer2Enforcement middleware. The module exposes
// three enforcer types:
//
//   - EntityOwner (6 routes, Entity="Terminal", Field="UserID"):
//       GET    /api/v1/terminals/:id/console
//       POST   /api/v1/terminals/:id/stop
//       POST   /api/v1/terminals/:id/sync
//       GET    /api/v1/terminals/:id/status
//       GET    /api/v1/terminals/:id/history
//       DELETE /api/v1/terminals/:id/history
//
//   - GroupRole (3 routes, MinRole="manager", Param="id"):
//       POST /api/v1/class-groups/:id/bulk-create-terminals
//       GET  /api/v1/class-groups/:id/command-history
//       GET  /api/v1/class-groups/:id/command-history-stats
//
//   - OrgRole (3 routes):
//       GET  /api/v1/organizations/:id/terminal-sessions         (member)
//       GET  /api/v1/organizations/:id/terminal-usage            (manager)
//       ALL  /api/v1/incus-ui/:backendId/*                       (member, Param=backendId)
//
// For each route we run four scenarios:
//
//   1. Outsider — Casbin Member with NO ownership / membership → expect 403
//   2. Insufficient role — only on routes with MinRole > member → expect 403
//   3. Authorized — user meets the declared rule → expect 200 (fake handler)
//   4. Admin bypass — Casbin Administrator → expect 200 (fake handler)
//
// The incus-ui route additionally gets a method-coverage test (GET and POST
// against the same registered path) to confirm Layer 2 honors the regex
// method matcher on its Lookup.
//
// The fake handler is a no-op that returns 200. If Layer 2 allows the request
// through, the handler fires and we see 200. If Layer 2 blocks, we see 403.

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
// Shared harness types — scoped to this audit so we don't couple to the
// shared mockMembershipChecker used elsewhere in the package.
// -----------------------------------------------------------------------------

// terminalsAuditMembershipChecker records group and org memberships for the
// terminals audit. Keys are "scopeID:userID" (where scopeID is either a
// group ID or an org ID depending on which map is looked up).
type terminalsAuditMembershipChecker struct {
	groupRoles map[string]string
	orgRoles   map[string]string
}

func (c *terminalsAuditMembershipChecker) CheckGroupRole(groupID, userID, minRole string) (bool, error) {
	role, ok := c.groupRoles[groupID+":"+userID]
	if !ok {
		return false, nil
	}
	return access.IsRoleAtLeast(role, minRole), nil
}

func (c *terminalsAuditMembershipChecker) CheckOrgRole(orgID, userID, minRole string) (bool, error) {
	role, ok := c.orgRoles[orgID+":"+userID]
	if !ok {
		return false, nil
	}
	return access.IsRoleAtLeast(role, minRole), nil
}

// terminalsAuditEntityLoader exposes a configurable owner map for Terminal
// entities. The EntityOwner enforcer calls GetOwnerField(entity, id, field)
// and the harness returns the value registered for (id, field).
type terminalsAuditEntityLoader struct {
	// owners maps "entityName:entityID:fieldName" -> owner value.
	owners map[string]string
}

func (l *terminalsAuditEntityLoader) GetOwnerField(entity, id, field string) (string, error) {
	key := entity + ":" + id + ":" + field
	if v, ok := l.owners[key]; ok {
		return v, nil
	}
	// Absent: return empty string (Layer 2 will compare to userID and deny).
	return "", nil
}

// -----------------------------------------------------------------------------
// Route catalog — mirrors src/terminalTrainer/routes/permissions.go.
// -----------------------------------------------------------------------------

type terminalsAuditRoute struct {
	method          string
	registeredPath  string
	requestPath     string
	// scopeID is the concrete URL-param value we use for :id / :backendId in
	// the request path. The checker / loader is keyed off this.
	scopeID         string
	ruleType        access.AccessRuleType
	minRole         string // only meaningful for GroupRole / OrgRole
	entity          string // only meaningful for EntityOwner
	field           string // only meaningful for EntityOwner
	paramName       string // "id" for most routes, "backendId" for incus-ui
}

var terminalsAuditEntityOwnerRoutes = []terminalsAuditRoute{
	{method: "GET", registeredPath: "/api/v1/terminals/:id/console", requestPath: "/api/v1/terminals/term-audit-console/console", scopeID: "term-audit-console", ruleType: access.EntityOwner, entity: "Terminal", field: "UserID", paramName: "id"},
	{method: "POST", registeredPath: "/api/v1/terminals/:id/stop", requestPath: "/api/v1/terminals/term-audit-stop/stop", scopeID: "term-audit-stop", ruleType: access.EntityOwner, entity: "Terminal", field: "UserID", paramName: "id"},
	{method: "POST", registeredPath: "/api/v1/terminals/:id/sync", requestPath: "/api/v1/terminals/term-audit-sync/sync", scopeID: "term-audit-sync", ruleType: access.EntityOwner, entity: "Terminal", field: "UserID", paramName: "id"},
	{method: "GET", registeredPath: "/api/v1/terminals/:id/status", requestPath: "/api/v1/terminals/term-audit-status/status", scopeID: "term-audit-status", ruleType: access.EntityOwner, entity: "Terminal", field: "UserID", paramName: "id"},
	{method: "GET", registeredPath: "/api/v1/terminals/:id/history", requestPath: "/api/v1/terminals/term-audit-histget/history", scopeID: "term-audit-histget", ruleType: access.EntityOwner, entity: "Terminal", field: "UserID", paramName: "id"},
	{method: "DELETE", registeredPath: "/api/v1/terminals/:id/history", requestPath: "/api/v1/terminals/term-audit-histdel/history", scopeID: "term-audit-histdel", ruleType: access.EntityOwner, entity: "Terminal", field: "UserID", paramName: "id"},
}

var terminalsAuditGroupRoutes = []terminalsAuditRoute{
	{method: "POST", registeredPath: "/api/v1/class-groups/:id/bulk-create-terminals", requestPath: "/api/v1/class-groups/grp-audit-bulk/bulk-create-terminals", scopeID: "grp-audit-bulk", ruleType: access.GroupRole, minRole: "manager", paramName: "id"},
	{method: "GET", registeredPath: "/api/v1/class-groups/:id/command-history", requestPath: "/api/v1/class-groups/grp-audit-hist/command-history", scopeID: "grp-audit-hist", ruleType: access.GroupRole, minRole: "manager", paramName: "id"},
	{method: "GET", registeredPath: "/api/v1/class-groups/:id/command-history-stats", requestPath: "/api/v1/class-groups/grp-audit-stats/command-history-stats", scopeID: "grp-audit-stats", ruleType: access.GroupRole, minRole: "manager", paramName: "id"},
}

var terminalsAuditOrgRoutes = []terminalsAuditRoute{
	{method: "GET", registeredPath: "/api/v1/organizations/:id/terminal-sessions", requestPath: "/api/v1/organizations/org-audit-sessions/terminal-sessions", scopeID: "org-audit-sessions", ruleType: access.OrgRole, minRole: "member", paramName: "id"},
	{method: "GET", registeredPath: "/api/v1/organizations/:id/terminal-usage", requestPath: "/api/v1/organizations/org-audit-usage/terminal-usage", scopeID: "org-audit-usage", ruleType: access.OrgRole, minRole: "manager", paramName: "id"},
	// Declared with regex method "(GET|POST|PUT|PATCH|DELETE)" and Param: "backendId".
	// See TestTerminalsLayer2_IncusUI_MethodCoverage for why this is tested
	// separately.
	{method: "(GET|POST|PUT|PATCH|DELETE)", registeredPath: "/api/v1/incus-ui/:backendId/*path", requestPath: "/api/v1/incus-ui/backend-audit/resources", scopeID: "backend-audit", ruleType: access.OrgRole, minRole: "member", paramName: "backendId"},
}

// setupTerminalsAuditRouter installs Layer 2 for a single route under audit.
// The caller passes a freshly-built checker/loader pair so each subtest
// controls exactly what memberships / owners are in scope.
func setupTerminalsAuditRouter(
	t *testing.T,
	route terminalsAuditRoute,
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

	// Mirror the real declaration from the terminals module.
	perm := access.RoutePermission{
		Path:   route.registeredPath,
		Method: route.method,
		Role:   "member",
		Access: access.AccessRule{
			Type:    route.ruleType,
			Param:   route.paramName,
			MinRole: route.minRole,
			Entity:  route.entity,
			Field:   route.field,
		},
	}
	access.RouteRegistry.Register("Terminals", perm)

	access.RegisterBuiltinEnforcers(loader, checker)

	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Header-driven user injection (mirrors payment_layer2_audit_test.go).
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
	// For the regex-method incus-ui route, we must attach the handler under
	// real HTTP verbs so Gin can actually route the request. We register the
	// union of allowed methods — the goal is to let the request reach
	// Layer2Enforcement, which performs the RouteRegistry.Lookup using the
	// actual request method.
	if strings.Contains(route.method, "|") {
		for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
			r.Handle(m, route.registeredPath, fake)
		}
	} else {
		r.Handle(route.method, route.registeredPath, fake)
	}
	return r
}

// doTerminalsAuditRequest issues an HTTP request with the given user headers.
func doTerminalsAuditRequest(r *gin.Engine, method, path, userID, roles string) *httptest.ResponseRecorder {
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

// requestMethodFor returns the actual HTTP verb to use when issuing the
// request. For the regex-method incus-ui route we deliberately issue a
// real verb (GET) — the registered "method" in the registry is a regex
// literal that Lookup will only match if it was stored as literal.
func requestMethodFor(route terminalsAuditRoute) string {
	if strings.Contains(route.method, "|") {
		return "GET"
	}
	return route.method
}

// -----------------------------------------------------------------------------
// Case 1: Outsider — Member with no ownership / no membership → 403
// -----------------------------------------------------------------------------

func TestTerminalsLayer2_Outsider_Denied(t *testing.T) {
	all := append([]terminalsAuditRoute{}, terminalsAuditEntityOwnerRoutes...)
	all = append(all, terminalsAuditGroupRoutes...)
	all = append(all, terminalsAuditOrgRoutes...)

	for _, route := range all {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			// Owner map intentionally leaves the terminal "owned by" a
			// different user so the outsider never matches.
			owners := map[string]string{}
			if route.ruleType == access.EntityOwner {
				owners["Terminal:"+route.scopeID+":UserID"] = "real-owner-user"
			}
			loader := &terminalsAuditEntityLoader{owners: owners}
			checker := &terminalsAuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			r := setupTerminalsAuditRouter(t, route, checker, loader)

			reqMethod := requestMethodFor(route)
			w := doTerminalsAuditRequest(r, reqMethod, route.requestPath, "outsider-user", "member")
			assert.Equal(t, http.StatusForbidden, w.Code,
				"outsider must be denied on %s %s (observed %d, body=%s)",
				reqMethod, route.requestPath, w.Code, w.Body.String())
		})
	}
}

// -----------------------------------------------------------------------------
// Case 2: Insufficient role — only for routes with MinRole > member.
// -----------------------------------------------------------------------------

func TestTerminalsLayer2_InsufficientRole_Denied(t *testing.T) {
	// Group routes (all require manager) + the manager-gated org route.
	var manager_gated []terminalsAuditRoute
	manager_gated = append(manager_gated, terminalsAuditGroupRoutes...)
	for _, r := range terminalsAuditOrgRoutes {
		if r.minRole == "manager" {
			manager_gated = append(manager_gated, r)
		}
	}

	for _, route := range manager_gated {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			checker := &terminalsAuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			switch route.ruleType {
			case access.GroupRole:
				checker.groupRoles[route.scopeID+":insufficient-user"] = "member"
			case access.OrgRole:
				checker.orgRoles[route.scopeID+":insufficient-user"] = "member"
			}
			loader := &terminalsAuditEntityLoader{owners: map[string]string{}}
			r := setupTerminalsAuditRouter(t, route, checker, loader)

			reqMethod := requestMethodFor(route)
			w := doTerminalsAuditRequest(r, reqMethod, route.requestPath, "insufficient-user", "member")
			assert.Equal(t, http.StatusForbidden, w.Code,
				"lower-than-manager must be denied on %s %s (observed %d)",
				reqMethod, route.requestPath, w.Code)
		})
	}
}

// -----------------------------------------------------------------------------
// Case 3: Authorized — user meets the rule → Layer 2 lets through.
// -----------------------------------------------------------------------------

func TestTerminalsLayer2_Authorized_Allowed(t *testing.T) {
	all := append([]terminalsAuditRoute{}, terminalsAuditEntityOwnerRoutes...)
	all = append(all, terminalsAuditGroupRoutes...)
	all = append(all, terminalsAuditOrgRoutes...)

	for _, route := range all {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			checker := &terminalsAuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			owners := map[string]string{}
			const authorizedUser = "authorized-user"

			switch route.ruleType {
			case access.EntityOwner:
				owners["Terminal:"+route.scopeID+":UserID"] = authorizedUser
			case access.GroupRole:
				checker.groupRoles[route.scopeID+":"+authorizedUser] = route.minRole
			case access.OrgRole:
				checker.orgRoles[route.scopeID+":"+authorizedUser] = route.minRole
			}

			loader := &terminalsAuditEntityLoader{owners: owners}
			r := setupTerminalsAuditRouter(t, route, checker, loader)

			reqMethod := requestMethodFor(route)
			w := doTerminalsAuditRequest(r, reqMethod, route.requestPath, authorizedUser, "member")
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"authorized user must not be blocked on %s %s (observed %d, body=%s)",
				reqMethod, route.requestPath, w.Code, w.Body.String())
			assert.Equal(t, http.StatusOK, w.Code,
				"fake handler should have returned 200 for %s %s (body=%s)",
				reqMethod, route.requestPath, w.Body.String())
		})
	}
}

// -----------------------------------------------------------------------------
// Case 4: Admin bypass — Casbin Administrator always allowed.
// -----------------------------------------------------------------------------

func TestTerminalsLayer2_AdminBypass_Allowed(t *testing.T) {
	all := append([]terminalsAuditRoute{}, terminalsAuditEntityOwnerRoutes...)
	all = append(all, terminalsAuditGroupRoutes...)
	all = append(all, terminalsAuditOrgRoutes...)

	for _, route := range all {
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			// No owners, no memberships — admin must still pass.
			loader := &terminalsAuditEntityLoader{owners: map[string]string{}}
			checker := &terminalsAuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			r := setupTerminalsAuditRouter(t, route, checker, loader)

			reqMethod := requestMethodFor(route)
			w := doTerminalsAuditRequest(r, reqMethod, route.requestPath, "admin-user", "administrator")
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"administrator must bypass %s enforcement on %s %s (observed %d)",
				route.ruleType, reqMethod, route.requestPath, w.Code)
			assert.Equal(t, http.StatusOK, w.Code,
				"admin must reach handler on %s %s", reqMethod, route.requestPath)
		})
	}
}

// -----------------------------------------------------------------------------
// Case 5: Wildcard incus-ui route must enforce across multiple methods.
//
// src/terminalTrainer/routes/permissions.go declares the route with
// Method: "(GET|POST|PUT|PATCH|DELETE)". The Layer2Enforcement middleware
// performs RouteRegistry.Lookup using the exact request method string. If
// the registry stores the literal regex as the method, concrete requests
// (GET /api/v1/incus-ui/...) will NOT match and Layer 2 silently passes
// through — a production gap.
//
// This test documents the expected behavior (both GET and POST must be
// denied for an outsider). If it fails, see the MR description: the fix is
// either to split the incus-ui declaration into five per-method entries or
// to teach RouteRegistry.Lookup to understand alternation.
// TODO(#264): currently failing — see MR description.
// -----------------------------------------------------------------------------

func TestTerminalsLayer2_IncusUI_MethodCoverage(t *testing.T) {
	// Find the incus-ui declaration in the catalog.
	var incus terminalsAuditRoute
	for _, r := range terminalsAuditOrgRoutes {
		if strings.Contains(r.registeredPath, "/incus-ui/") {
			incus = r
			break
		}
	}
	if incus.registeredPath == "" {
		t.Fatal("incus-ui route missing from catalog")
	}

	for _, verb := range []string{"GET", "POST"} {
		t.Run(verb+"_outsider_denied", func(t *testing.T) {
			loader := &terminalsAuditEntityLoader{owners: map[string]string{}}
			checker := &terminalsAuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			r := setupTerminalsAuditRouter(t, incus, checker, loader)

			w := doTerminalsAuditRequest(r, verb, incus.requestPath, "outsider-user", "member")
			// TODO(#264): currently failing — see MR description.
			assert.Equal(t, http.StatusForbidden, w.Code,
				"outsider must be denied on %s %s (observed %d); if 200, Layer 2 is not matching the regex method declaration",
				verb, incus.requestPath, w.Code)
		})
	}
}

// -----------------------------------------------------------------------------
// Case 6: /console sharing regression guard.
//
// terminalController.hasTerminalAccess (src/terminalTrainer/routes/
// terminalController.go:139) intentionally lets non-owners (e.g. a group
// owner whose members include the terminal owner) connect to /console.
// Layer 2 EntityOwner is strict: only the literal UserID match is allowed.
// If both checks now run in sequence, a group-owner viewer is rejected by
// Layer 2 before the controller's sharing logic gets a chance to run. This
// test documents the observable behavior so the team can decide whether to
// keep the restriction or relax it via a custom enforcer.
//
// NOTE: We DO NOT fix this here. The test asserts what currently happens;
// if it ever flips, the test flags it for review.
// -----------------------------------------------------------------------------

func TestTerminalsLayer2_Console_SharingConflict_Documented(t *testing.T) {
	// Use the /console route.
	var consoleRoute terminalsAuditRoute
	for _, r := range terminalsAuditEntityOwnerRoutes {
		if strings.HasSuffix(r.registeredPath, "/console") {
			consoleRoute = r
			break
		}
	}

	// Terminal owned by the student; the group owner is a different user.
	loader := &terminalsAuditEntityLoader{
		owners: map[string]string{
			"Terminal:" + consoleRoute.scopeID + ":UserID": "student-user",
		},
	}
	checker := &terminalsAuditMembershipChecker{
		groupRoles: map[string]string{},
		orgRoles:   map[string]string{},
	}
	r := setupTerminalsAuditRouter(t, consoleRoute, checker, loader)

	// Group owner attempts to connect to the student's terminal console.
	w := doTerminalsAuditRequest(r, "GET", consoleRoute.requestPath, "group-owner-user", "member")

	// Layer 2 should reject because UserID mismatch, even though the
	// controller's hasTerminalAccess() would have allowed it. This is the
	// documented "sharing regression" — not fixed here.
	assert.Equal(t, http.StatusForbidden, w.Code,
		"Layer 2 EntityOwner rejects non-owner access to /console even though controller-level sharing logic permits it; if this ever flips, sharing semantics have changed")
}
