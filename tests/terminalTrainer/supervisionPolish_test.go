package terminalTrainer_tests

// RED tests for the supervision framework-readiness polish batch (issue #425
// follow-up). Two behaviours are pinned here that the current code gets wrong:
//
// A — CRITICAL (review C1): forked admin predicate. supervisionController.go
//     derives isAdmin via a private isAdminFromRoles that matches ONLY the exact
//     literal "administrator". The canonical access.IsAdmin
//     (src/auth/access/helpers.go) is case-insensitive AND accepts the "admin"
//     alias (Casdoor emits both forms). So a platform operator whose role is
//     "admin" or "Administrator" is admitted everywhere in OCF EXCEPT
//     supervision — a silent authorization hole where an admin is wrongly denied.
//
//     SEAM PINNED: supervision admin detection must route through ONE function —
//     access.IsAdmin — at the handler layer (GetGroupTerminalSessions :120 and
//     SuperviseSession :198). isAdminFromRoles is deleted by the fix, so these
//     tests deliberately do NOT reference it: they drive the exported
//     GetGroupTerminalSessions handler through a gin router with userRoles
//     carrying alias/case variants and assert the non-member admin bypass admits
//     the group listing (200). GetGroupTerminalSessions is chosen over
//     SuperviseSession because it is a plain JSON handler (no WS upgrade), yet it
//     exercises the identical roles→isAdmin derivation feeding both call sites.
//
// B — review M1: missing `released` audit when the hand is held at disconnect.
//     Today a trainer who takes the hand and then simply disconnects (stream torn
//     down, no explicit release_hand frame) leaves an audit trail of
//     take_hand ... stopped but NEVER released — compliance cannot bound who held
//     interactive control and until when.
//
//     SEAM PINNED (assumed by the item-B tests): EndSupervision gains a trailing
//     `handHeld bool`. When the supervision window ends WITH the hand still held,
//     it emits AuditEventSupervisionReleased immediately BEFORE
//     AuditEventSupervisionStopped; when the hand was not held it emits only
//     Stopped (unchanged from today). SuperviseSession's teardown passes its final
//     `promoted` state as handHeld. Threading the flag through EndSupervision (a
//     pure DB/audit-service boundary) rather than burying the emission in the WS
//     teardown goroutine is what makes the compliance guarantee testable — this is
//     a testability-driven design nudge, not an incidental choice.
//     NOTE: this changes EndSupervision's signature; the pre-existing
//     TestSupervision_EndAudit_UsesStoppedEvent call in supervision_test.go is
//     updated to pass handHeld=false to match.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditModels "soli/formations/src/audit/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// --- A. Forked admin predicate: admin aliases/case must bypass (review C1) ----

// TestSupervision_AdminRoleAliasesBypassGroupCheck pins that a platform operator
// who is NOT a member of the group is admitted to the group's supervision
// listing for EVERY spelling of the admin role Casdoor may emit — the "admin"
// alias and any case variant — not just the exact literal "administrator".
//
// Currently RED for the alias/case rows: the forked isAdminFromRoles matches only
// "administrator" verbatim, so a caller with role "admin" / "Administrator" /
// "ADMIN" falls through to the (failing) callerManagesGroup check and is wrongly
// denied with 403. The "administrator" row is the control that must stay green:
// the fix (routing through access.IsAdmin) keeps the exact form working.
func TestSupervision_AdminRoleAliasesBypassGroupCheck(t *testing.T) {
	cases := []struct {
		name string
		role string
	}{
		{"lowercase alias admin", "admin"},
		{"capitalized administrator", "Administrator"},
		{"uppercase alias ADMIN", "ADMIN"},
		{"exact administrator (control, green today)", "administrator"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)

			// Group owned by trainer-A with learner-A holding an active session.
			// The admin caller has NO membership in this group, so a 200 can only
			// come from the admin bypass (never from callerManagesGroup).
			group, sessionID := newSupervisedSession(t, db, "group-A", "trainer-A", "learner-A")

			ctrl := terminalController.NewTerminalController(db)

			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("userId", "admin-user")
				c.Set("userRoles", []string{tc.role})
				c.Next()
			})
			router.GET("/class-groups/:id/terminal-sessions", ctrl.GetGroupTerminalSessions)

			req := httptest.NewRequest(http.MethodGet, "/class-groups/"+group.ID.String()+"/terminal-sessions", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code,
				"a platform admin (role %q) must bypass the group check and see the listing; body=%s",
				tc.role, w.Body.String())

			var rows []map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
			present := make(map[string]bool)
			for _, r := range rows {
				if sid, ok := r["session_id"].(string); ok {
					present[sid] = true
				}
			}
			assert.True(t, present[sessionID],
				"the admin listing must include the group member's active session")
		})
	}
}

// TestSupervision_NonAdminNonMemberStillDenied is the negative guard: a caller
// who is neither an admin nor a manager of the group must still be denied (403).
// It pins that the C1 fix (broadening admin detection) does NOT accidentally
// admit ordinary roles — green today and must stay green after the fix.
func TestSupervision_NonAdminNonMemberStillDenied(t *testing.T) {
	db := setupTestDB(t)

	group, _ := newSupervisedSession(t, db, "group-A", "trainer-A", "learner-A")

	ctrl := terminalController.NewTerminalController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "stranger")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/class-groups/:id/terminal-sessions", ctrl.GetGroupTerminalSessions)

	req := httptest.NewRequest(http.MethodGet, "/class-groups/"+group.ID.String()+"/terminal-sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a non-admin who does not manage the group must be denied; body=%s", w.Body.String())
}

// --- B. `released` audit when the hand is held at disconnect (review M1) ------

// TestSupervision_EndWithHandHeld_EmitsReleasedThenStopped pins the compliance
// guarantee: when supervision ends while the trainer STILL holds the interactive
// hand (a plain disconnect with no release_hand frame), the audit trail records a
// terminal.supervision.released event BEFORE the terminal.supervision.stopped
// event that bounds the window. Without this, "who held control, and until when"
// is unanswerable from the trail.
//
// Currently RED: EndSupervision emits only Stopped and has no handHeld parameter.
func TestSupervision_EndWithHandHeld_EmitsReleasedThenStopped(t *testing.T) {
	db := setupTestDB(t)

	trainer := "trainer-A"
	group, sessionID := newSupervisedSession(t, db, "group-A", trainer, "learner-A")
	audit := &mockSupervisionAudit{}

	err := terminalController.EndSupervision(db, audit, trainer, false, sessionID, group.ID.String(), true)
	require.NoError(t, err)

	releasedIdx, stoppedIdx := -1, -1
	for i, e := range audit.logged {
		switch e.EventType {
		case auditModels.AuditEventSupervisionReleased:
			releasedIdx = i
		case auditModels.AuditEventSupervisionStopped:
			stoppedIdx = i
		}
	}

	require.NotEqual(t, -1, releasedIdx,
		"ending supervision while the hand is held MUST emit a released event so control can be bounded")
	require.NotEqual(t, -1, stoppedIdx,
		"the supervision window must still be closed with a stopped event")
	assert.Less(t, releasedIdx, stoppedIdx,
		"the hand must be recorded as released BEFORE the window is recorded as stopped")
}

// TestSupervision_EndWithoutHand_EmitsOnlyStopped pins the paired negative: when
// the hand was NOT held at disconnect (the ordinary observe-only case, or after
// an explicit release), EndSupervision emits ONLY the stopped event — no spurious
// released row. This keeps the trail honest and guards the item-B fix from
// emitting a phantom release for every teardown.
func TestSupervision_EndWithoutHand_EmitsOnlyStopped(t *testing.T) {
	db := setupTestDB(t)

	trainer := "trainer-A"
	group, sessionID := newSupervisedSession(t, db, "group-A", trainer, "learner-A")
	audit := &mockSupervisionAudit{}

	err := terminalController.EndSupervision(db, audit, trainer, false, sessionID, group.ID.String(), false)
	require.NoError(t, err)

	require.Len(t, audit.logged, 1,
		"ending supervision with no hand held must emit exactly one audit entry")
	assert.Equal(t, auditModels.AuditEventSupervisionStopped, audit.logged[0].EventType,
		"the sole event must be the stopped window bound")
}
