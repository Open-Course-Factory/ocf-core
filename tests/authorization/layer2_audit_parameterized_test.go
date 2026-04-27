package authorization_tests

// Parameterized Layer 2 authorization audit suite.
//
// This file provides a single table-driven harness that runs the four core
// enforcement scenarios (Outsider, InsufficientRole, Authorized, AdminBypass)
// for all routes across the auth, scenarios, organizations, and payment
// modules.  The route catalogs remain in their per-module files (so reviewers
// see what each module declares) — this file only contains the generic harness
// types and the parameterized test functions that consume those catalogs.
//
// Four enforcement scenarios per route (where applicable):
//
//  1. Outsider — Casbin Member with no ownership / no group or org membership
//     → expect 403.
//  2. InsufficientRole — user holds a role one rung below the required minRole
//     (only for GroupRole / OrgRole routes with minRole > "member") → 403.
//  3. Authorized — user satisfies the declared rule → expect 200.
//  4. AdminBypass — Casbin Administrator → expect 200 regardless of rule.
//
// Rules that skip specific sub-scenarios:
//
//   - EntityOwner: no minRole concept → InsufficientRole is skipped.
//   - AdminOnly: a non-admin member IS the outsider case; InsufficientRole
//     is also skipped (AdminOnly has no role hierarchy, just admin / non-admin).
//   - SelfScoped: documentation-only; enforcement runs in the controller,
//     not in Layer 2.  Skipped entirely in enforcement tests (covered only
//     by registry-parity tests in the per-module files).
//
// Generic helpers are prefixed "layer2Audit" to avoid collisions with the
// per-module harness helpers that were removed from the originating files.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
)

// =============================================================================
// Generic harness types
// =============================================================================

// layer2AuditMembershipChecker handles both group and org role checks in a
// single struct so it can serve all four modules without duplication.
// Keys are "scopeID:userID" for both group and org maps.
type layer2AuditMembershipChecker struct {
	groupRoles map[string]string // "groupID:userID" -> role name
	orgRoles   map[string]string // "orgID:userID"   -> role name
}

func (c *layer2AuditMembershipChecker) CheckGroupRole(groupID, userID, minRole string) (bool, error) {
	role, ok := c.groupRoles[groupID+":"+userID]
	if !ok {
		return false, nil
	}
	return access.IsRoleAtLeast(role, minRole), nil
}

func (c *layer2AuditMembershipChecker) CheckOrgRole(orgID, userID, minRole string) (bool, error) {
	role, ok := c.orgRoles[orgID+":"+userID]
	if !ok {
		return false, nil
	}
	return access.IsRoleAtLeast(role, minRole), nil
}

// layer2AuditEntityLoader stores entity ownership by "entity:id:field".
// Returns ("", nil) for absent keys — both paths (empty owner vs. error)
// converge on a 403 from the EntityOwner enforcer.
type layer2AuditEntityLoader struct {
	owners map[string]string
}

func (l *layer2AuditEntityLoader) GetOwnerField(entity, id, field string) (string, error) {
	if v, ok := l.owners[entity+":"+id+":"+field]; ok {
		return v, nil
	}
	return "", nil
}

// =============================================================================
// Unified route descriptor
// =============================================================================

// layer2AuditRoute describes one route entry in the cross-module audit catalog.
// Fields map onto all supported rule types; unused fields are zero-valued.
type layer2AuditRoute struct {
	// module is the registry category (e.g. "Scenarios", "Organizations").
	module string
	// method is the concrete HTTP verb ("GET", "POST", "PATCH", "DELETE", "PUT").
	method string
	// registeredPath is the Gin route pattern (e.g. "/api/v1/groups/:groupId/...").
	registeredPath string
	// requestPath is the concrete URL used in HTTP requests (all params filled in).
	requestPath string
	// scopeID is the value injected into the relevant URL param (":id", ":groupId",
	// etc.) and used as the key when populating the membership checker / entity loader.
	// Empty for routes where no scope param is needed (e.g. AdminOnly without :id).
	scopeID string
	// ruleType is the Layer 2 access rule type (AdminOnly, EntityOwner, GroupRole, OrgRole).
	ruleType access.AccessRuleType
	// minRole is the minimum role required (GroupRole / OrgRole only).
	minRole string
	// entity is the entity name for EntityOwner lookups (e.g. "ScenarioSession").
	entity string
	// field is the ownership field for EntityOwner lookups (e.g. "UserID").
	field string
	// paramName is the URL parameter name used by GroupRole / OrgRole enforcers
	// (e.g. "groupId", "id").  EntityOwner always uses "id" in production.
	paramName string
	// skipInsufficientRoleTest suppresses the InsufficientRole sub-scenario for
	// rule types that have no meaningful "one rung below" concept
	// (EntityOwner, AdminOnly).
	skipInsufficientRoleTest bool
}

// =============================================================================
// Generic setup / request helpers
// =============================================================================

// setupLayer2AuditRouter installs Layer 2 with the production declaration for
// exactly one route, then returns a Gin engine with a fake 200 handler wired
// on that route.  Each call starts from a clean registry + enforcer state and
// registers a cleanup to restore that state.
func setupLayer2AuditRouter(
	t *testing.T,
	route layer2AuditRoute,
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

	// Build the access rule mirroring production declarations.
	rule := access.AccessRule{
		Type:    route.ruleType,
		MinRole: route.minRole,
		Entity:  route.entity,
		Field:   route.field,
	}
	// GroupRole and OrgRole honor rule.Param; EntityOwner defaults to "id".
	switch route.ruleType {
	case access.GroupRole, access.OrgRole:
		rule.Param = route.paramName
	}

	// AdminOnly routes use "administrator" as the RBAC role; everything else
	// uses "member" (Casbin sees member for all real users).
	casbinRole := "member"
	if route.ruleType == access.AdminOnly {
		casbinRole = "administrator"
	}

	access.RouteRegistry.Register(route.module,
		access.RoutePermission{
			Path:   route.registeredPath,
			Method: route.method,
			Role:   casbinRole,
			Access: rule,
		},
	)

	access.RegisterBuiltinEnforcers(loader, checker)

	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Inject userId / userRoles from test-specific headers before Layer 2 runs.
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

	// Fake handler: if Layer 2 lets the request through, we see 200.
	fake := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "layer2-allowed"})
	}
	r.Handle(route.method, route.registeredPath, fake)
	return r
}

// doLayer2AuditRequest fires a request against the test router with the given
// user identity headers and returns the recorded response.
func doLayer2AuditRequest(r *gin.Engine, method, path, userID, roles string) *httptest.ResponseRecorder {
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

// =============================================================================
// Cross-module merged catalog
// =============================================================================

// allLayer2AuditRoutes returns the merged catalog across all four modules.
// Each module contributes only its Layer 2-enforced routes (AdminOnly,
// EntityOwner, GroupRole, OrgRole).  SelfScoped and Public routes are excluded
// — they are covered by per-module registry parity tests.
//
// Route catalogs are declared in their respective per-module files:
//   - auth_layer2_audit_test.go      (authAuditAdminRoutes)
//   - scenarios_layer2_audit_test.go (scenariosAuditEntity/Group/OrgRoutes)
//   - organizations_layer2_audit_test.go (organizationsAuditOrg/AdminRoutes)
//   - payment_layer2_audit_test.go   (paymentAuditRoutes)
//
// Adapters below convert per-module structs into layer2AuditRoute.
func allLayer2AuditRoutes() []layer2AuditRoute {
	var routes []layer2AuditRoute
	routes = append(routes, adaptAuthAdminRoutes()...)
	routes = append(routes, adaptScenariosRoutes()...)
	routes = append(routes, adaptOrganizationsRoutes()...)
	routes = append(routes, adaptPaymentRoutes()...)
	return routes
}

// adaptAuthAdminRoutes converts authAuditAdminRoutes (auth module) to the
// unified struct.  SelfScoped routes are omitted — they have no Layer 2
// enforcement and are covered by TestAuthLayer2_RegistryDeclaresEveryAuditedRoute.
func adaptAuthAdminRoutes() []layer2AuditRoute {
	out := make([]layer2AuditRoute, 0, len(authAuditAdminRoutes))
	for _, r := range authAuditAdminRoutes {
		out = append(out, layer2AuditRoute{
			module:                   r.module,
			method:                   r.method,
			registeredPath:           r.registeredPath,
			requestPath:              r.requestPath,
			scopeID:                  "", // AdminOnly routes have no scope param
			ruleType:                 r.ruleType,
			skipInsufficientRoleTest: true, // AdminOnly: no role hierarchy
		})
	}
	return out
}

// adaptScenariosRoutes converts all three scenarios sub-catalogs.
func adaptScenariosRoutes() []layer2AuditRoute {
	all := allScenariosAuditRoutes()
	out := make([]layer2AuditRoute, 0, len(all))
	for _, r := range all {
		out = append(out, layer2AuditRoute{
			module:                   "Scenarios",
			method:                   r.method,
			registeredPath:           r.registeredPath,
			requestPath:              r.requestPath,
			scopeID:                  r.scopeID,
			ruleType:                 r.ruleType,
			minRole:                  r.minRole,
			entity:                   r.entity,
			field:                    r.field,
			paramName:                r.paramName,
			skipInsufficientRoleTest: r.ruleType == access.EntityOwner,
		})
	}
	return out
}

// adaptOrganizationsRoutes converts organizations sub-catalogs.
func adaptOrganizationsRoutes() []layer2AuditRoute {
	all := allOrganizationsAuditRoutes()
	out := make([]layer2AuditRoute, 0, len(all))
	for _, r := range all {
		out = append(out, layer2AuditRoute{
			module:                   "Organizations",
			method:                   r.method,
			registeredPath:           r.registeredPath,
			requestPath:              r.requestPath,
			scopeID:                  r.scopeID,
			ruleType:                 r.ruleType,
			minRole:                  r.minRole,
			paramName:                r.paramName,
			skipInsufficientRoleTest: r.ruleType == access.AdminOnly,
		})
	}
	return out
}

// adaptPaymentRoutes converts paymentAuditRoutes. All payment routes are OrgRole
// with Param="id"; scopeID maps from the per-module orgID field.
func adaptPaymentRoutes() []layer2AuditRoute {
	out := make([]layer2AuditRoute, 0, len(paymentAuditRoutes))
	for _, r := range paymentAuditRoutes {
		out = append(out, layer2AuditRoute{
			module:         "Organization Subscriptions",
			method:         r.method,
			registeredPath: r.registeredPath,
			requestPath:    r.requestPath,
			scopeID:        r.orgID,
			ruleType:       access.OrgRole,
			minRole:        r.minRole,
			paramName:      "id",
		})
	}
	return out
}

// =============================================================================
// Parameterized enforcement tests
// =============================================================================

// TestLayer2Audit_Outsider_Denied asserts that a Casbin Member with no
// ownership / no group or org membership is denied (403) on every
// Layer 2-enforced route across all four modules.
func TestLayer2Audit_Outsider_Denied(t *testing.T) {
	for _, route := range allLayer2AuditRoutes() {
		route := route // capture
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			checker := &layer2AuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			owners := map[string]string{}
			if route.ruleType == access.EntityOwner {
				// Entity is owned by someone else — outsider should never match.
				owners[route.entity+":"+route.scopeID+":"+route.field] = "real-owner-user"
			}
			loader := &layer2AuditEntityLoader{owners: owners}
			r := setupLayer2AuditRouter(t, route, checker, loader)

			w := doLayer2AuditRequest(r, route.method, route.requestPath, "outsider-user", "member")
			assert.Equal(t, http.StatusForbidden, w.Code,
				"outsider must be denied on %s %s (ruleType=%s, observed=%d, body=%s)",
				route.method, route.requestPath, route.ruleType, w.Code, w.Body.String())
		})
	}
}

// TestLayer2Audit_InsufficientRole_Denied asserts that a user holding a role
// one rung below the required minRole is denied (403).  Routes without a
// meaningful role hierarchy (EntityOwner, AdminOnly) are skipped.
func TestLayer2Audit_InsufficientRole_Denied(t *testing.T) {
	for _, route := range allLayer2AuditRoutes() {
		route := route // capture
		if route.skipInsufficientRoleTest {
			continue
		}
		// Only applies to GroupRole / OrgRole routes where minRole > "member".
		if route.ruleType != access.GroupRole && route.ruleType != access.OrgRole {
			continue
		}
		if route.minRole == "member" {
			continue // no insufficient-role case for member-gated routes
		}

		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			// Pick the role one rung below the declared minimum.
			var insufficientRole string
			switch route.minRole {
			case "manager":
				insufficientRole = "member"
			case "owner":
				insufficientRole = "manager"
			default:
				t.Fatalf("unexpected minRole %q for %s %s — extend this switch",
					route.minRole, route.method, route.registeredPath)
			}

			checker := &layer2AuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			switch route.ruleType {
			case access.GroupRole:
				checker.groupRoles[route.scopeID+":insufficient-user"] = insufficientRole
			case access.OrgRole:
				checker.orgRoles[route.scopeID+":insufficient-user"] = insufficientRole
			}
			loader := &layer2AuditEntityLoader{owners: map[string]string{}}
			r := setupLayer2AuditRouter(t, route, checker, loader)

			w := doLayer2AuditRequest(r, route.method, route.requestPath, "insufficient-user", "member")
			assert.Equal(t, http.StatusForbidden, w.Code,
				"%s role must be denied on %s-gated %s %s (observed=%d)",
				insufficientRole, route.minRole, route.method, route.requestPath, w.Code)
		})
	}
}

// TestLayer2Audit_Authorized_Allowed asserts that a user who satisfies the
// declared access rule reaches the handler (200).
func TestLayer2Audit_Authorized_Allowed(t *testing.T) {
	const authorizedUser = "authorized-user"

	for _, route := range allLayer2AuditRoutes() {
		route := route // capture
		// AdminOnly "authorized" case is covered by AdminBypass (only admins pass).
		if route.ruleType == access.AdminOnly {
			continue
		}

		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			checker := &layer2AuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			owners := map[string]string{}

			switch route.ruleType {
			case access.EntityOwner:
				owners[route.entity+":"+route.scopeID+":"+route.field] = authorizedUser
			case access.GroupRole:
				checker.groupRoles[route.scopeID+":"+authorizedUser] = route.minRole
			case access.OrgRole:
				checker.orgRoles[route.scopeID+":"+authorizedUser] = route.minRole
			}

			loader := &layer2AuditEntityLoader{owners: owners}
			r := setupLayer2AuditRouter(t, route, checker, loader)

			w := doLayer2AuditRequest(r, route.method, route.requestPath, authorizedUser, "member")
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"authorized user must not be blocked on %s %s (ruleType=%s, observed=%d, body=%s)",
				route.method, route.requestPath, route.ruleType, w.Code, w.Body.String())
			assert.Equal(t, http.StatusOK, w.Code,
				"fake handler must return 200 for authorized user on %s %s (body=%s)",
				route.method, route.requestPath, w.Body.String())
		})
	}
}

// TestLayer2Audit_AdminBypass_Allowed asserts that a Casbin Administrator
// bypasses every Layer 2 enforcement rule and reaches the handler (200).
func TestLayer2Audit_AdminBypass_Allowed(t *testing.T) {
	for _, route := range allLayer2AuditRoutes() {
		route := route // capture
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			// Admin has no membership / ownership — bypass must still allow.
			checker := &layer2AuditMembershipChecker{
				groupRoles: map[string]string{},
				orgRoles:   map[string]string{},
			}
			loader := &layer2AuditEntityLoader{owners: map[string]string{}}
			r := setupLayer2AuditRouter(t, route, checker, loader)

			w := doLayer2AuditRequest(r, route.method, route.requestPath, "admin-user", "administrator")
			assert.NotEqual(t, http.StatusForbidden, w.Code,
				"administrator must bypass %s enforcement on %s %s (observed=%d)",
				route.ruleType, route.method, route.requestPath, w.Code)
			assert.Equal(t, http.StatusOK, w.Code,
				"admin must reach handler on %s %s (body=%s)",
				route.method, route.requestPath, w.Body.String())
		})
	}
}

// =============================================================================
// Generic structural guards (cross-module)
// =============================================================================

// TestLayer2Audit_RegistryDeclaresEveryAuditedRoute cross-walks the merged
// catalog against the RouteRegistry.  Every (method, path) pair in the
// catalog must be findable via RouteRegistry.Lookup — a missing entry means
// Layer 2 silently passes the route through without consulting the rule.
func TestLayer2Audit_RegistryDeclaresEveryAuditedRoute(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	// Register every route exactly as production would.
	for _, route := range allLayer2AuditRoutes() {
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
		casbinRole := "member"
		if route.ruleType == access.AdminOnly {
			casbinRole = "administrator"
		}
		access.RouteRegistry.Register(route.module,
			access.RoutePermission{
				Path:   route.registeredPath,
				Method: route.method,
				Role:   casbinRole,
				Access: rule,
			},
		)
	}

	for _, route := range allLayer2AuditRoutes() {
		route := route // capture
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			perm, found := access.RouteRegistry.Lookup(route.method, route.registeredPath)
			assert.True(t, found,
				"RouteRegistry.Lookup must find a declaration for %s %s (module=%s) — a missing entry means Layer 2 silently passes the route through",
				route.method, route.registeredPath, route.module)
			if found {
				assert.Equal(t, route.ruleType, perm.Access.Type,
					"declared access rule type must match catalog for %s %s",
					route.method, route.registeredPath)
			}
		})
	}
}

// TestLayer2Audit_NoRegexMethodInCatalog verifies that every catalog entry
// uses a single concrete HTTP verb (GET, POST, etc.) rather than a regex
// alternation like "(GET|POST|PATCH|DELETE)".  RouteRegistry.Lookup does
// exact method+path string matching, so alternation-style methods silently
// bypass Layer 2 enforcement (see #265, MR !180).
func TestLayer2Audit_NoRegexMethodInCatalog(t *testing.T) {
	allowed := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	}

	for _, route := range allLayer2AuditRoutes() {
		route := route // capture
		t.Run(route.method+" "+route.registeredPath, func(t *testing.T) {
			assert.True(t, allowed[route.method],
				"catalog route %s %s declares a non-canonical HTTP method — Layer 2 Lookup is exact-match, regex/alternation methods silently bypass enforcement (see #265, MR !180)",
				route.method, route.registeredPath)
			assert.NotContains(t, route.method, "|",
				"alternation in method string %q for %s would silently bypass Layer 2 (see #265, MR !180)",
				route.method, route.registeredPath)
		})
	}
}
