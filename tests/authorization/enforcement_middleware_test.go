package authorization_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	access "soli/formations/src/auth/access"
)

// ---------------------------------------------------------------------------
// Test 1: IsAdmin canonical helper (case-insensitive)
// ---------------------------------------------------------------------------
// The casbin.IsAdmin(roles) function is the single canonical source of truth
// for checking administrator status. It must handle all case variations
// because Casdoor returns mixed-case role names.

func TestIsAdmin_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name     string
		roles    []string
		expected bool
	}{
		{
			name:     "lowercase administrator",
			roles:    []string{"administrator"},
			expected: true,
		},
		{
			name:     "capitalized Administrator (Casdoor format)",
			roles:    []string{"Administrator"},
			expected: true,
		},
		{
			name:     "uppercase ADMINISTRATOR",
			roles:    []string{"ADMINISTRATOR"},
			expected: true,
		},
		{
			name:     "member only returns false",
			roles:    []string{"member"},
			expected: false,
		},
		{
			name:     "admin alias from Casdoor",
			roles:    []string{"admin"},
			expected: true,
		},
		{
			name:     "empty roles returns false",
			roles:    []string{},
			expected: false,
		},
		{
			name:     "nil roles returns false",
			roles:    nil,
			expected: false,
		},
		{
			name:     "mixed roles with administrator present",
			roles:    []string{"member", "administrator"},
			expected: true,
		},
		{
			name:     "unrelated roles only",
			roles:    []string{"teacher", "student", "trainer"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// IsAdmin is the NEW canonical helper in the casbin package.
			// It does not exist yet — this test should fail to compile.
			result := access.IsAdmin(tt.roles)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ---------------------------------------------------------------------------
// Test 2: RouteRegistry.Lookup(method, path)
// ---------------------------------------------------------------------------
// The Lookup method allows the enforcement middleware to find the
// RoutePermission for a given HTTP method + path, so it knows which
// Layer 2 access rule to enforce.

func TestRouteRegistry_Lookup_Hit(t *testing.T) {
	// Use a fresh registry to avoid test pollution
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	// Register some routes
	access.RouteRegistry.Register("auth", access.RoutePermission{
		Path:       "/api/v1/auth/me",
		Method:     "GET",
		Role: "member",
		Access:     access.AccessRule{Type: access.SelfScoped},
	})
	access.RouteRegistry.Register("admin", access.RoutePermission{
		Path:       "/api/v1/admin/security/policies",
		Method:     "GET",
		Role: "administrator",
		Access:     access.AccessRule{Type: access.AdminOnly},
	})
	access.RouteRegistry.Register("terminal", access.RoutePermission{
		Path:       "/api/v1/terminals/start-composed-session",
		Method:     "POST",
		Role: "member",
		Access:     access.AccessRule{Type: access.SelfScoped},
	})

	tests := []struct {
		name           string
		method         string
		path           string
		expectFound    bool
		expectAccess   access.AccessRuleType
		expectCasbin   string
		expectCategory string
	}{
		{
			name:           "lookup auth/me GET",
			method:         "GET",
			path:           "/api/v1/auth/me",
			expectFound:    true,
			expectAccess:   access.SelfScoped,
			expectCasbin:   "member",
			expectCategory: "auth",
		},
		{
			name:           "lookup admin policies GET",
			method:         "GET",
			path:           "/api/v1/admin/security/policies",
			expectFound:    true,
			expectAccess:   access.AdminOnly,
			expectCasbin:   "administrator",
			expectCategory: "admin",
		},
		{
			name:           "lookup terminal start-composed-session POST",
			method:         "POST",
			path:           "/api/v1/terminals/start-composed-session",
			expectFound:    true,
			expectAccess:   access.SelfScoped,
			expectCasbin:   "member",
			expectCategory: "terminal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Lookup is the NEW method on routeRegistry.
			// It does not exist yet — this test should fail to compile.
			perm, found := access.RouteRegistry.Lookup(tt.method, tt.path)
			assert.True(t, found, "expected route to be found")
			assert.Equal(t, tt.expectAccess, perm.Access.Type)
			assert.Equal(t, tt.expectCasbin, perm.Role)
			assert.Equal(t, tt.expectCategory, perm.Category)
		})
	}
}

func TestRouteRegistry_Lookup_Miss(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	// Register one route
	access.RouteRegistry.Register("auth", access.RoutePermission{
		Path:       "/api/v1/auth/me",
		Method:     "GET",
		Role: "member",
		Access:     access.AccessRule{Type: access.SelfScoped},
	})

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "wrong method",
			method: "POST",
			path:   "/api/v1/auth/me",
		},
		{
			name:   "wrong path",
			method: "GET",
			path:   "/api/v1/nonexistent",
		},
		{
			name:   "both wrong",
			method: "DELETE",
			path:   "/api/v1/something/else",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, found := access.RouteRegistry.Lookup(tt.method, tt.path)
			assert.False(t, found, "expected route NOT to be found for %s %s", tt.method, tt.path)
		})
	}
}

func TestRouteRegistry_Lookup_MultipleMethodsSamePath(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	// Register GET and POST on the same path with different access rules
	access.RouteRegistry.Register("terminals", access.RoutePermission{
		Path:       "/api/v1/terminals",
		Method:     "GET",
		Role: "member",
		Access:     access.AccessRule{Type: access.Public},
	})
	access.RouteRegistry.Register("terminals", access.RoutePermission{
		Path:       "/api/v1/terminals",
		Method:     "POST",
		Role: "administrator",
		Access:     access.AccessRule{Type: access.AdminOnly},
	})

	// GET should return Public
	getPerm, getFound := access.RouteRegistry.Lookup("GET", "/api/v1/terminals")
	assert.True(t, getFound)
	assert.Equal(t, access.Public, getPerm.Access.Type)
	assert.Equal(t, "member", getPerm.Role)

	// POST should return AdminOnly
	postPerm, postFound := access.RouteRegistry.Lookup("POST", "/api/v1/terminals")
	assert.True(t, postFound)
	assert.Equal(t, access.AdminOnly, postPerm.Access.Type)
	assert.Equal(t, "administrator", postPerm.Role)
}

// ---------------------------------------------------------------------------
// Mock implementations for Layer 2 enforcement middleware tests
// ---------------------------------------------------------------------------

// mockEntityLoader implements access.EntityLoader for testing.
type mockEntityLoader struct {
	ownerFieldValue string
	err             error
}

func (m *mockEntityLoader) GetOwnerField(entityName string, entityID string, fieldName string) (string, error) {
	return m.ownerFieldValue, m.err
}

// mockMembershipChecker implements access.MembershipChecker for testing.
type mockMembershipChecker struct {
	groupRoleResult bool
	groupRoleErr    error
	orgRoleResult   bool
	orgRoleErr      error
}

func (m *mockMembershipChecker) CheckGroupRole(groupID string, userID string, minRole string) (bool, error) {
	return m.groupRoleResult, m.groupRoleErr
}

func (m *mockMembershipChecker) CheckOrgRole(orgID string, userID string, minRole string) (bool, error) {
	return m.orgRoleResult, m.orgRoleErr
}

// setupTestRouter creates a Gin engine with userId/userRoles injected from
// request headers, followed by the given middleware.
func setupTestRouter(middleware gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Inject userId and userRoles before the enforcement middleware
	r.Use(func(c *gin.Context) {
		c.Set("userId", c.GetHeader("X-Test-UserId"))
		c.Set("userRoles", strings.Split(c.GetHeader("X-Test-Roles"), ","))
		c.Next()
	})
	r.Use(middleware)
	return r
}

// okHandler is a simple 200 OK handler for test routes.
func okHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Test 3: Layer 2 Enforcement Middleware
// ---------------------------------------------------------------------------

func TestLayer2_AdminOnly_Allowed(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	// Register the route as AdminOnly
	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:       "/api/v1/test/admin-action",
		Method:     "POST",
		Role: "administrator",
		Access:     access.AccessRule{Type: access.AdminOnly},
	})

	loader := &mockEntityLoader{}
	checker := &mockMembershipChecker{}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.POST("/api/v1/test/admin-action", okHandler)

	req := httptest.NewRequest("POST", "/api/v1/test/admin-action", nil)
	req.Header.Set("X-Test-UserId", "user1")
	req.Header.Set("X-Test-Roles", "administrator")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "administrator should be allowed on AdminOnly route")
}

func TestLayer2_AdminOnly_Denied(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:       "/api/v1/test/admin-action",
		Method:     "POST",
		Role: "administrator",
		Access:     access.AccessRule{Type: access.AdminOnly},
	})

	loader := &mockEntityLoader{}
	checker := &mockMembershipChecker{}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.POST("/api/v1/test/admin-action", okHandler)

	req := httptest.NewRequest("POST", "/api/v1/test/admin-action", nil)
	req.Header.Set("X-Test-UserId", "user1")
	req.Header.Set("X-Test-Roles", "member")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "member should be denied on AdminOnly route")
}

func TestLayer2_EntityOwner_Allowed(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:       "/api/v1/test/:id/edit",
		Method:     "PATCH",
		Role: "member",
		Access: access.AccessRule{
			Type:   access.EntityOwner,
			Entity: "TestEntity",
			Field:  "UserID",
		},
	})

	// Mock returns "user1" as the owner
	loader := &mockEntityLoader{ownerFieldValue: "user1"}
	checker := &mockMembershipChecker{}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.PATCH("/api/v1/test/:id/edit", okHandler)

	req := httptest.NewRequest("PATCH", "/api/v1/test/entity123/edit", nil)
	req.Header.Set("X-Test-UserId", "user1")
	req.Header.Set("X-Test-Roles", "member")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "entity owner should be allowed")
}

func TestLayer2_EntityOwner_Denied(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:       "/api/v1/test/:id/edit",
		Method:     "PATCH",
		Role: "member",
		Access: access.AccessRule{
			Type:   access.EntityOwner,
			Entity: "TestEntity",
			Field:  "UserID",
		},
	})

	// Mock returns "user1" as the owner, but the requester is "user2"
	loader := &mockEntityLoader{ownerFieldValue: "user1"}
	checker := &mockMembershipChecker{}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.PATCH("/api/v1/test/:id/edit", okHandler)

	req := httptest.NewRequest("PATCH", "/api/v1/test/entity123/edit", nil)
	req.Header.Set("X-Test-UserId", "user2")
	req.Header.Set("X-Test-Roles", "member")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "non-owner should be denied")
}

func TestLayer2_EntityOwner_AdminBypass(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:       "/api/v1/test/:id/edit",
		Method:     "PATCH",
		Role: "member",
		Access: access.AccessRule{
			Type:   access.EntityOwner,
			Entity: "TestEntity",
			Field:  "UserID",
		},
	})

	// Mock returns "user1" as the owner, but requester is "user2" with admin role
	loader := &mockEntityLoader{ownerFieldValue: "user1"}
	checker := &mockMembershipChecker{}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.PATCH("/api/v1/test/:id/edit", okHandler)

	req := httptest.NewRequest("PATCH", "/api/v1/test/entity123/edit", nil)
	req.Header.Set("X-Test-UserId", "user2")
	req.Header.Set("X-Test-Roles", "administrator")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "administrator should bypass entity owner check")
}

// ---------------------------------------------------------------------------
// Regression: EntityOwner enforcer must honor rule.Param (issue #266)
// ---------------------------------------------------------------------------
// The EntityOwner enforcer in src/auth/access/enforcement_middleware.go (line 112)
// reads `entityID := ctx.Param("id")` — a hardcoded literal that ignores
// `rule.Param`. The sibling enforcers GroupRole (line 135) and OrgRole (line 158)
// both correctly read `ctx.Param(rule.Param)`.
//
// This is a hidden footgun: a route declaring EntityOwner with a custom param
// name (e.g., `Param: "resourceId"` on `/api/v1/test-resources/:resourceId`)
// silently misroutes the lookup — `ctx.Param("id")` returns "" and the
// owner check fails for the legitimate owner.
//
// Expected behavior: when rule.Param is non-empty, the enforcer must use
// `ctx.Param(rule.Param)`. When rule.Param is empty, it must default to "id"
// for backward-compatibility with existing declarations.
//
// This test exercises the non-default branch with `Param: "resourceId"` on a
// route whose only path param is `:resourceId` (no `:id`). It MUST currently
// FAIL (owner gets 403 instead of 200) and pass once the enforcer is fixed.
func TestEnforcementMiddleware_EntityOwner_HonorsRuleParam_Issue266(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	// Register an EntityOwner rule that uses a non-"id" path parameter name.
	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:   "/api/v1/test-resources/:resourceId",
		Method: "PATCH",
		Role:   "member",
		Access: access.AccessRule{
			Type:   access.EntityOwner,
			Entity: "TestResource",
			Field:  "UserID",
			Param:  "resourceId",
		},
	})

	// The loader returns the correct owner ("user1") ONLY when looked up
	// with the right entity ID ("res-42"). Any other ID (notably the empty
	// string the buggy enforcer would send) returns "" — which fails the
	// owner comparison and yields 403. This makes the primary 200/403
	// assertion a faithful signal of the bug, not just an artefact of the
	// loader stub.
	loader := &idAwareEntityLoader{
		ownersByID: map[string]string{"res-42": "user1"},
	}
	checker := &mockMembershipChecker{}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.PATCH("/api/v1/test-resources/:resourceId", okHandler)

	req := httptest.NewRequest("PATCH", "/api/v1/test-resources/res-42", nil)
	req.Header.Set("X-Test-UserId", "user1")
	req.Header.Set("X-Test-Roles", "member")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Primary assertion: owner must reach the handler (200).
	// Currently fails because the enforcer reads ctx.Param("id") which is
	// empty for this route (only :resourceId exists). With the empty ID
	// the loader returns "" (not "user1"), the comparison fails, and the
	// enforcer aborts with 403. With the planned fix (read
	// ctx.Param(rule.Param) when non-empty, default to "id" otherwise) the
	// entityID is "res-42", the loader returns "user1", the request reaches
	// okHandler.
	assert.Equal(t, http.StatusOK, w.Code,
		"EntityOwner enforcer must honor rule.Param — owner of resource via :resourceId should be allowed (#266)")

	// Secondary assertion: the loader must have been invoked with the
	// path parameter value, NOT an empty string. This pin-points the bug
	// even if the primary assertion is masked by future loader changes.
	assert.Equal(t, "res-42", loader.lastEntityID,
		"EntityOwner enforcer must pass the value of rule.Param to the loader, not the value of a hardcoded \"id\" param (#266)")
}

// idAwareEntityLoader returns the configured owner only when the lookup ID
// matches one in its map; for unknown IDs (including the empty string the
// buggy enforcer would send) it returns an empty owner. It also records the
// last entityID it was called with so tests can pin-point routing bugs.
// Used by the #266 regression test.
type idAwareEntityLoader struct {
	ownersByID     map[string]string
	err            error
	lastEntityName string
	lastEntityID   string
	lastFieldName  string
}

func (m *idAwareEntityLoader) GetOwnerField(entityName string, entityID string, fieldName string) (string, error) {
	m.lastEntityName = entityName
	m.lastEntityID = entityID
	m.lastFieldName = fieldName
	if m.err != nil {
		return "", m.err
	}
	return m.ownersByID[entityID], nil
}

func TestLayer2_GroupRole_Allowed(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:       "/api/v1/test/groups/:groupId/manage",
		Method:     "POST",
		Role: "member",
		Access: access.AccessRule{
			Type:    access.GroupRole,
			Param:   "groupId",
			MinRole: "manager",
		},
	})

	loader := &mockEntityLoader{}
	checker := &mockMembershipChecker{groupRoleResult: true}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.POST("/api/v1/test/groups/:groupId/manage", okHandler)

	req := httptest.NewRequest("POST", "/api/v1/test/groups/group42/manage", nil)
	req.Header.Set("X-Test-UserId", "user1")
	req.Header.Set("X-Test-Roles", "member")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "user with sufficient group role should be allowed")
}

func TestLayer2_GroupRole_Denied(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:       "/api/v1/test/groups/:groupId/manage",
		Method:     "POST",
		Role: "member",
		Access: access.AccessRule{
			Type:    access.GroupRole,
			Param:   "groupId",
			MinRole: "manager",
		},
	})

	loader := &mockEntityLoader{}
	checker := &mockMembershipChecker{groupRoleResult: false}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.POST("/api/v1/test/groups/:groupId/manage", okHandler)

	req := httptest.NewRequest("POST", "/api/v1/test/groups/group42/manage", nil)
	req.Header.Set("X-Test-UserId", "user1")
	req.Header.Set("X-Test-Roles", "member")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "user without sufficient group role should be denied")
}

func TestLayer2_OrgRole_Allowed(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:   "/api/v1/test/orgs/:orgId/manage",
		Method: "POST",
		Role:   "member",
		Access: access.AccessRule{
			Type:    access.OrgRole,
			Param:   "orgId",
			MinRole: "manager",
		},
	})

	loader := &mockEntityLoader{}
	checker := &mockMembershipChecker{orgRoleResult: true}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.POST("/api/v1/test/orgs/:orgId/manage", okHandler)

	req := httptest.NewRequest("POST", "/api/v1/test/orgs/org42/manage", nil)
	req.Header.Set("X-Test-UserId", "user1")
	req.Header.Set("X-Test-Roles", "member")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "user with sufficient org role should be allowed")
}

func TestLayer2_OrgRole_Denied(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:   "/api/v1/test/orgs/:orgId/manage",
		Method: "POST",
		Role:   "member",
		Access: access.AccessRule{
			Type:    access.OrgRole,
			Param:   "orgId",
			MinRole: "manager",
		},
	})

	loader := &mockEntityLoader{}
	checker := &mockMembershipChecker{orgRoleResult: false}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.POST("/api/v1/test/orgs/:orgId/manage", okHandler)

	req := httptest.NewRequest("POST", "/api/v1/test/orgs/org42/manage", nil)
	req.Header.Set("X-Test-UserId", "user1")
	req.Header.Set("X-Test-Roles", "member")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "user without sufficient org role should be denied")
}

func TestLayer2_Public_Passthrough(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	access.RouteRegistry.Register("test", access.RoutePermission{
		Path:       "/api/v1/test/public-resource",
		Method:     "GET",
		Role: "member",
		Access:     access.AccessRule{Type: access.Public},
	})

	loader := &mockEntityLoader{}
	checker := &mockMembershipChecker{}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.GET("/api/v1/test/public-resource", okHandler)

	req := httptest.NewRequest("GET", "/api/v1/test/public-resource", nil)
	req.Header.Set("X-Test-UserId", "anyuser")
	req.Header.Set("X-Test-Roles", "member")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "public route should allow any authenticated user")
}

func TestLayer2_UnregisteredRoute_Passthrough(t *testing.T) {
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	defer func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	}()

	// Deliberately do NOT register any route in the registry

	loader := &mockEntityLoader{}
	checker := &mockMembershipChecker{}
	access.RegisterBuiltinEnforcers(loader, checker)
	mw := access.Layer2Enforcement()
	r := setupTestRouter(mw)
	r.GET("/api/v1/test/unregistered", okHandler)

	req := httptest.NewRequest("GET", "/api/v1/test/unregistered", nil)
	req.Header.Set("X-Test-UserId", "anyuser")
	req.Header.Set("X-Test-Roles", "member")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "unregistered route should pass through for backwards compatibility")
}
