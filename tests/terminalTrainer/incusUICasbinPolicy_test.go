package terminalTrainer_tests

import (
	"testing"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestEnforcer creates a Casbin enforcer with ocf-core's own model file (not a
// copy, which would keep testing the old matcher after the real one changes) and
// in-memory policies.
func newTestEnforcer(t *testing.T) *casbin.Enforcer {
	t.Helper()
	m, err := model.NewModelFromFile("../../src/configuration/keymatch_model.conf")
	require.NoError(t, err, "failed to parse Casbin model")
	e, err := casbin.NewEnforcer(m)
	require.NoError(t, err, "failed to create Casbin enforcer")
	return e
}

// TestIncusUICasbinPolicy_MemberCanAccessAllIncusUIRoutes verifies that the
// corrected Casbin policy (using "*" wildcard and including PATCH) allows
// the member role to access all real Incus UI proxy URLs.
func TestIncusUICasbinPolicy_MemberCanAccessAllIncusUIRoutes(t *testing.T) {
	e := newTestEnforcer(t)

	// The CORRECT policy — matches permissions.go:102
	_, err := e.AddPolicy("member", "/api/v1/incus-ui/:backendId/*", "(GET|POST|PUT|PATCH|DELETE)")
	require.NoError(t, err)

	testCases := []struct {
		name   string
		url    string
		method string
	}{
		{"auth cookie setup", "/api/v1/incus-ui/_auth/cookie", "POST"},
		{"iframe initial load", "/api/v1/incus-ui/backend1/ui/", "GET"},
		{"Incus API call (GET)", "/api/v1/incus-ui/backend1/1.0/instances", "GET"},
		{"asset request", "/api/v1/incus-ui/backend1/ui/assets/index.js", "GET"},
		{"WebSocket upgrade", "/api/v1/incus-ui/backend1/1.0/events", "GET"},
		{"instance config (PATCH)", "/api/v1/incus-ui/backend1/1.0/instances/foo", "PATCH"},
		{"instance config (PUT)", "/api/v1/incus-ui/backend1/1.0/instances/foo", "PUT"},
		{"instance delete (DELETE)", "/api/v1/incus-ui/backend1/1.0/instances/foo", "DELETE"},
		{"instance create (POST)", "/api/v1/incus-ui/backend1/1.0/instances", "POST"},
		{"deep nested path", "/api/v1/incus-ui/backend1/1.0/instances/foo/state", "PUT"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, err := e.Enforce("member", tc.url, tc.method)
			assert.NoError(t, err)
			assert.True(t, allowed,
				"member should be allowed %s %s via incus-ui proxy", tc.method, tc.url)
		})
	}
}

// TestIncusUICasbinPolicy_DoesNotLeakToOtherRoutes verifies that the incus-ui
// wildcard policy does not accidentally grant access to unrelated API routes.
func TestIncusUICasbinPolicy_DoesNotLeakToOtherRoutes(t *testing.T) {
	e := newTestEnforcer(t)

	_, err := e.AddPolicy("member", "/api/v1/incus-ui/:backendId/*", "(GET|POST|PUT|PATCH|DELETE)")
	require.NoError(t, err)

	testCases := []struct {
		name   string
		url    string
		method string
	}{
		{"terminals endpoint", "/api/v1/terminals/start-session", "POST"},
		{"users endpoint", "/api/v1/users/me", "GET"},
		{"organizations endpoint", "/api/v1/organizations", "GET"},
		{"admin security", "/api/v1/admin/security/policies", "GET"},
		{"incus-ui without backend", "/api/v1/incus-ui", "GET"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, err := e.Enforce("member", tc.url, tc.method)
			assert.NoError(t, err)
			assert.False(t, allowed,
				"incus-ui policy must NOT grant access to %s", tc.url)
		})
	}
}
