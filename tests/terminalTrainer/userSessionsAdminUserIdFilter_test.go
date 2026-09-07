package terminalTrainer_tests

// Tests for the `user_id` widening on GET /terminals/user-sessions (issue #470).
//
// Passing `user_id` lets a platform administrator list another user's
// sessions. The handler must decide "is this caller an administrator" through
// the codebase's one canonical predicate, access.IsAdmin, rather than an
// inlined string comparison — otherwise the two drift (IsAdmin is
// case-insensitive and accepts the "admin" alias; the inline loop was not).

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetUserSessionsUserIdFilterAcceptsEveryAdminSpellingIsAdminAccepts pins
// that the handler and access.IsAdmin agree: every role spelling IsAdmin
// accepts widens the list to the target user's sessions.
func TestGetUserSessionsUserIdFilterAcceptsEveryAdminSpellingIsAdminAccepts(t *testing.T) {
	db := freshTestDB(t)
	targetSession := newOrgTerminal(t, db, "learner-470", nil)

	for _, roles := range [][]string{{"administrator"}, {"Administrator"}, {"admin"}} {
		w := getUserSessionsAs(t, db, "ops-470", roles, "?user_id=learner-470")

		require.Equal(t, http.StatusOK, w.Code, "roles=%v body=%s", roles, w.Body.String())
		assert.Equal(t, []string{targetSession}, sessionIDsIn(t, w), "roles=%v", roles)
	}
}

// TestGetUserSessionsUserIdFilterRefusesNonAdmin pins the negative side of the
// same rule: a regular member passing user_id is refused with 403, whatever
// sessions the target has.
func TestGetUserSessionsUserIdFilterRefusesNonAdmin(t *testing.T) {
	db := freshTestDB(t)
	newOrgTerminal(t, db, "learner-470", nil)

	w := getUserSessionsAs(t, db, "student-470", []string{"member"}, "?user_id=learner-470")

	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
}
