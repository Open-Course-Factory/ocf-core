package terminalTrainer_tests

import (
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keymatchModelConf is the exact Casbin model used by ocf-core
// (from src/configuration/keymatch_model.conf).
const keymatchModelConf = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = (g(r.sub, p.sub) || r.sub == p.sub) && keyMatch2(r.obj, p.obj) && regexMatch(r.act, p.act)
`

// newTestEnforcer creates a Casbin enforcer with the keymatch model and in-memory policies.
func newTestEnforcer(t *testing.T) *casbin.Enforcer {
	t.Helper()
	m, err := model.NewModelFromString(keymatchModelConf)
	require.NoError(t, err, "failed to parse Casbin model")
	e, err := casbin.NewEnforcer(m)
	require.NoError(t, err, "failed to create Casbin enforcer")
	return e
}

// TestIncusUICasbinPolicy_StarPath_NeverMatchesRealURLs is a regression test
// proving that using "*path" instead of "*" in a keyMatch2 Casbin policy breaks
// matching for ALL real URLs. keyMatch2 only treats "/*" as a wildcard (→ "/.*"),
// but "/*path" becomes "/.*path" — requiring the URL to end with literal "path".
//
// This was the root cause of the "missing Authorization header" error for non-admin
// users accessing the Incus UI: Casbin denied the auth cookie setup POST (403),
// the frontend ignored the error, and the iframe loaded without the cookie.
func TestIncusUICasbinPolicy_StarPath_NeverMatchesRealURLs(t *testing.T) {
	e := newTestEnforcer(t)

	// The OLD (buggy) policy — DO NOT use *path in keyMatch2 patterns
	_, err := e.AddPolicy("member", "/api/v1/incus-ui/:backendId/*path", "(GET|POST|PUT|DELETE)")
	require.NoError(t, err)

	testCases := []struct {
		name   string
		url    string
		method string
	}{
		{"auth cookie setup", "/api/v1/incus-ui/_auth/cookie", "POST"},
		{"iframe initial load", "/api/v1/incus-ui/backend1/ui/", "GET"},
		{"Incus API call", "/api/v1/incus-ui/backend1/1.0/instances", "GET"},
		{"asset request", "/api/v1/incus-ui/backend1/ui/assets/index.js", "GET"},
		{"WebSocket upgrade", "/api/v1/incus-ui/backend1/1.0/events", "GET"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, err := e.Enforce("member", tc.url, tc.method)
			assert.NoError(t, err)
			assert.False(t, allowed,
				"Regression: *path pattern must NOT match %s — keyMatch2 turns /*path "+
					"into /.*path regex which requires URLs to end with literal 'path'",
				tc.url)
		})
	}
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

// TestIncusUICasbinPolicy_MissingPATCH_Regression is a regression test verifying
// that omitting PATCH from the method list would block Incus API config updates.
func TestIncusUICasbinPolicy_MissingPATCH_Regression(t *testing.T) {
	e := newTestEnforcer(t)

	// Policy WITHOUT PATCH — would break instance configuration
	_, err := e.AddPolicy("member", "/api/v1/incus-ui/:backendId/*", "(GET|POST|PUT|DELETE)")
	require.NoError(t, err)

	allowed, err := e.Enforce("member", "/api/v1/incus-ui/backend1/1.0/instances/foo", "PATCH")
	assert.NoError(t, err)
	assert.False(t, allowed,
		"Regression: policy without PATCH must not allow PATCH requests")
}
