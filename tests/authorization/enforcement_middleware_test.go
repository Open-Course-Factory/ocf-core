package authorization_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	casbinUtils "soli/formations/src/auth/casbin"
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
			result := casbinUtils.IsAdmin(tt.roles)
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
	casbinUtils.RouteRegistry.Reset()
	defer casbinUtils.RouteRegistry.Reset()

	// Register some routes
	casbinUtils.RouteRegistry.Register("auth", casbinUtils.RoutePermission{
		Path:       "/api/v1/auth/me",
		Method:     "GET",
		CasbinRole: "member",
		Access:     casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
	})
	casbinUtils.RouteRegistry.Register("admin", casbinUtils.RoutePermission{
		Path:       "/api/v1/admin/security/policies",
		Method:     "GET",
		CasbinRole: "administrator",
		Access:     casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
	})
	casbinUtils.RouteRegistry.Register("terminal", casbinUtils.RoutePermission{
		Path:       "/api/v1/terminals/start-session",
		Method:     "POST",
		CasbinRole: "member",
		Access:     casbinUtils.AccessRule{Type: casbinUtils.Public},
	})

	tests := []struct {
		name           string
		method         string
		path           string
		expectFound    bool
		expectAccess   casbinUtils.AccessRuleType
		expectCasbin   string
		expectCategory string
	}{
		{
			name:           "lookup auth/me GET",
			method:         "GET",
			path:           "/api/v1/auth/me",
			expectFound:    true,
			expectAccess:   casbinUtils.SelfScoped,
			expectCasbin:   "member",
			expectCategory: "auth",
		},
		{
			name:           "lookup admin policies GET",
			method:         "GET",
			path:           "/api/v1/admin/security/policies",
			expectFound:    true,
			expectAccess:   casbinUtils.AdminOnly,
			expectCasbin:   "administrator",
			expectCategory: "admin",
		},
		{
			name:           "lookup terminal start-session POST",
			method:         "POST",
			path:           "/api/v1/terminals/start-session",
			expectFound:    true,
			expectAccess:   casbinUtils.Public,
			expectCasbin:   "member",
			expectCategory: "terminal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Lookup is the NEW method on routeRegistry.
			// It does not exist yet — this test should fail to compile.
			perm, found := casbinUtils.RouteRegistry.Lookup(tt.method, tt.path)
			assert.True(t, found, "expected route to be found")
			assert.Equal(t, tt.expectAccess, perm.Access.Type)
			assert.Equal(t, tt.expectCasbin, perm.CasbinRole)
			assert.Equal(t, tt.expectCategory, perm.Category)
		})
	}
}

func TestRouteRegistry_Lookup_Miss(t *testing.T) {
	casbinUtils.RouteRegistry.Reset()
	defer casbinUtils.RouteRegistry.Reset()

	// Register one route
	casbinUtils.RouteRegistry.Register("auth", casbinUtils.RoutePermission{
		Path:       "/api/v1/auth/me",
		Method:     "GET",
		CasbinRole: "member",
		Access:     casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
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
			_, found := casbinUtils.RouteRegistry.Lookup(tt.method, tt.path)
			assert.False(t, found, "expected route NOT to be found for %s %s", tt.method, tt.path)
		})
	}
}

func TestRouteRegistry_Lookup_MultipleMethodsSamePath(t *testing.T) {
	casbinUtils.RouteRegistry.Reset()
	defer casbinUtils.RouteRegistry.Reset()

	// Register GET and POST on the same path with different access rules
	casbinUtils.RouteRegistry.Register("terminals", casbinUtils.RoutePermission{
		Path:       "/api/v1/terminals",
		Method:     "GET",
		CasbinRole: "member",
		Access:     casbinUtils.AccessRule{Type: casbinUtils.Public},
	})
	casbinUtils.RouteRegistry.Register("terminals", casbinUtils.RoutePermission{
		Path:       "/api/v1/terminals",
		Method:     "POST",
		CasbinRole: "administrator",
		Access:     casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
	})

	// GET should return Public
	getPerm, getFound := casbinUtils.RouteRegistry.Lookup("GET", "/api/v1/terminals")
	assert.True(t, getFound)
	assert.Equal(t, casbinUtils.Public, getPerm.Access.Type)
	assert.Equal(t, "member", getPerm.CasbinRole)

	// POST should return AdminOnly
	postPerm, postFound := casbinUtils.RouteRegistry.Lookup("POST", "/api/v1/terminals")
	assert.True(t, postFound)
	assert.Equal(t, casbinUtils.AdminOnly, postPerm.Access.Type)
	assert.Equal(t, "administrator", postPerm.CasbinRole)
}
