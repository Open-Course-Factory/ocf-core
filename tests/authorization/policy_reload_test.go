package authorization_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// Authorization reads the in-memory policy; it must not re-read casbin_rule on
// every request. Every policy write goes through the enforcer, which updates
// memory and the table together, so a per-request reload only re-reads what
// memory already holds — the whole table, on every authenticated call.
//
// The table is emptied behind the enforcer's back: a request that still passes
// proves the middleware did not reload it.
func TestAuthorizationDoesNotReloadPolicyPerRequest(t *testing.T) {
	stack := newMatrixStack(t)
	world := stack.freshWorld(t)
	req := get(route("/api/v1/organizations/%s/members", ecole))(world)

	code, body := stack.do(t, mxOrgOwner, req)
	require.Equal(t, http.StatusOK, code, "precondition: the owner lists the members: %s", body)

	require.NoError(t, stack.casbinDB.Exec("DELETE FROM casbin_rule").Error)

	code, body = stack.do(t, mxOrgOwner, req)
	require.Equal(t, http.StatusOK, code, "the request re-read casbin_rule: %s", body)
}
