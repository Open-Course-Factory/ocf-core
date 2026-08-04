package terminalTrainer_tests

// Tests for the `group_id` filter on GET /terminals/user-sessions (issue #464).
//
// The endpoint is SelfScoped by default. Passing `group_id` widens it to the
// group's members — a genuine scope widening, so the negative cases below matter
// as much as the feature itself.
//
// The authorization rule is NOT reimplemented here: the handler must route
// through the same "may this caller see this group's sessions" predicate the
// supervision wall uses (terminalController.MayListGroupSessions, extracted from
// ListGroupSupervisionSessions in src/terminalTrainer/routes/supervision.go), so
// the two surfaces can never drift. These tests pin the observable consequences
// of that reuse: manager+/owner/admin widen, everyone else is refused with the
// same 403 the supervision wall returns, and the org-context visibility rule
// still bounds what a teacher sees.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// --- Fixtures ----------------------------------------------------------------

// newGroupWithMemberSessions creates an ACTIVE class-group owned by ownerUserID
// in a fresh organization, records ownerUserID as an owner-role membership and
// every learner as a member-role membership, and gives each of them one running
// terminal stamped with the group's organization (the only context in which a
// teacher may see another user's session — see SupervisableByGroupOrgScope).
// Returns the group and a user id -> session id map.
func newGroupWithMemberSessions(t *testing.T, db *gorm.DB, groupName, ownerUserID string, learnerUserIDs ...string) (*groupModels.ClassGroup, map[string]string) {
	t.Helper()

	orgID := uuid.New()
	group := &groupModels.ClassGroup{
		Name:           groupName,
		DisplayName:    groupName,
		OwnerUserID:    ownerUserID,
		IsActive:       true,
		MaxMembers:     50,
		OrganizationID: &orgID,
	}
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	createTestGroupMember(t, db, group.ID, ownerUserID, groupModels.GroupMemberRoleOwner)
	sessions := map[string]string{ownerUserID: newOrgTerminal(t, db, ownerUserID, &orgID)}
	for _, learner := range learnerUserIDs {
		createTestGroupMember(t, db, group.ID, learner, groupModels.GroupMemberRoleMember)
		sessions[learner] = newOrgTerminal(t, db, learner, &orgID)
	}
	return group, sessions
}

// newOrgTerminal creates one running terminal for userID launched in orgID
// (nil = a personal session) and returns its session id.
func newOrgTerminal(t *testing.T, db *gorm.DB, userID string, orgID *uuid.UUID) string {
	t.Helper()

	userKey, err := createTestUserKey(db, userID+"-"+uuid.NewString())
	require.NoError(t, err)

	terminal := &models.Terminal{
		SessionID:         "session-" + uuid.NewString(),
		UserID:            userID,
		Name:              "Terminal of " + userID,
		State:             models.StateRunning,
		ExpiresAt:         time.Now().Add(time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		OrganizationID:    orgID,
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(terminal).Error)
	return terminal.SessionID
}

// getUserSessionsAs calls GET /terminals/user-sessions as callerUserID with the
// given roles and query string, returning the recorded response.
func getUserSessionsAs(t *testing.T, db *gorm.DB, callerUserID string, roles []string, query string) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", callerUserID)
		c.Set("userRoles", roles)
		c.Next()
	})
	router.GET("/terminals/user-sessions", terminalController.NewTerminalController(db).GetUserSessions)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/terminals/user-sessions"+query, nil))
	return w
}

// sessionIDsIn decodes the response body and returns the session_id of each row.
func sessionIDsIn(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows), "body=%s", w.Body.String())

	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		id, _ := row["session_id"].(string)
		ids = append(ids, id)
	}
	return ids
}

// --- 1. The widening: a group manager sees the group's sessions --------------

// TestGetUserSessionsGroupFilterAsGroupOwnerReturnsMemberSessions pins the
// feature: the teacher who owns the group gets every member's session, not only
// their own.
func TestGetUserSessionsGroupFilterAsGroupOwnerReturnsMemberSessions(t *testing.T) {
	db := freshTestDB(t)
	group, sessions := newGroupWithMemberSessions(t, db, "group-owner-widening", "trainer-A", "learner-A", "learner-B")

	w := getUserSessionsAs(t, db, "trainer-A", []string{"member"}, "?group_id="+group.ID.String())

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.ElementsMatch(t,
		[]string{sessions["trainer-A"], sessions["learner-A"], sessions["learner-B"]},
		sessionIDsIn(t, w),
		"a group owner filtering by group_id must see every active member's session")
}

// TestGetUserSessionsGroupFilterAsGroupManagerReturnsMemberSessions pins that
// the manager role widens too — the rule is manager+, not owner-only, exactly as
// the supervision wall defines it.
func TestGetUserSessionsGroupFilterAsGroupManagerReturnsMemberSessions(t *testing.T) {
	db := freshTestDB(t)
	group, sessions := newGroupWithMemberSessions(t, db, "group-manager-widening", "trainer-A", "learner-A")

	manager := "manager-A"
	createTestGroupMember(t, db, group.ID, manager, groupModels.GroupMemberRoleManager)
	managerSession := newOrgTerminal(t, db, manager, group.OrganizationID)

	w := getUserSessionsAs(t, db, manager, []string{"member"}, "?group_id="+group.ID.String())

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.ElementsMatch(t,
		[]string{sessions["trainer-A"], sessions["learner-A"], managerSession},
		sessionIDsIn(t, w),
		"a manager-role member must widen to the group just like the owner does")
}

// TestGetUserSessionsGroupFilterAsAdminReturnsMemberSessions pins the platform
// administrator bypass, consistent with every other group-scoped read.
func TestGetUserSessionsGroupFilterAsAdminReturnsMemberSessions(t *testing.T) {
	db := freshTestDB(t)
	group, sessions := newGroupWithMemberSessions(t, db, "group-admin-widening", "trainer-A", "learner-A")

	w := getUserSessionsAs(t, db, "platform-admin", []string{"administrator"}, "?group_id="+group.ID.String())

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.ElementsMatch(t,
		[]string{sessions["trainer-A"], sessions["learner-A"]},
		sessionIDsIn(t, w),
		"a platform administrator bypasses the group role check")
}

// --- 2. The negatives: no widening without the role -------------------------

// TestGetUserSessionsGroupFilterAsPlainMemberIsRefused is the load-bearing
// negative: a student who genuinely belongs to the group must NOT be able to
// read their classmates' sessions by passing the group id they can legitimately
// see in the UI.
func TestGetUserSessionsGroupFilterAsPlainMemberIsRefused(t *testing.T) {
	db := freshTestDB(t)
	group, sessions := newGroupWithMemberSessions(t, db, "group-student-refused", "trainer-A", "learner-A", "learner-B")

	w := getUserSessionsAs(t, db, "learner-A", []string{"member"}, "?group_id="+group.ID.String())

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a member-role learner must be refused the group widening (same 403 the supervision wall returns)")
	assert.NotContains(t, w.Body.String(), sessions["learner-B"],
		"a refused response must never leak a classmate's session id")
}

// TestGetUserSessionsGroupFilterAsNonMemberIsRefused pins that a stranger to the
// group gets nothing — the IDOR case where the group id is simply guessed.
func TestGetUserSessionsGroupFilterAsNonMemberIsRefused(t *testing.T) {
	db := freshTestDB(t)
	group, sessions := newGroupWithMemberSessions(t, db, "group-stranger-refused", "trainer-A", "learner-A")
	newOrgTerminal(t, db, "outsider", group.OrganizationID)

	w := getUserSessionsAs(t, db, "outsider", []string{"member"}, "?group_id="+group.ID.String())

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a caller with no membership in the group must be refused")
	assert.NotContains(t, w.Body.String(), sessions["learner-A"],
		"a refused response must never leak a group member's session id")
}

// TestGetUserSessionsGroupFilterOfInactiveGroupIsRefused pins that an archived
// class-group grants no authority, mirroring callerManagesAnyGroup's active-group
// restriction.
func TestGetUserSessionsGroupFilterOfInactiveGroupIsRefused(t *testing.T) {
	db := freshTestDB(t)
	group, _ := newGroupWithMemberSessions(t, db, "group-inactive-refused", "trainer-A", "learner-A")
	require.NoError(t, db.Model(&groupModels.ClassGroup{}).Where("id = ?", group.ID).Update("is_active", false).Error)

	w := getUserSessionsAs(t, db, "trainer-A", []string{"member"}, "?group_id="+group.ID.String())

	assert.Equal(t, http.StatusForbidden, w.Code,
		"an inactive class-group must not widen the listing, even for its owner")
}

// --- 3. Malformed input: no widening, no 500 --------------------------------

// TestGetUserSessionsGarbageGroupFilterIsRefusedNotCrashed pins that an
// unparseable group id is refused the same way an unauthorized one is — the
// supervision wall's behaviour — and never reaches a 500.
func TestGetUserSessionsGarbageGroupFilterIsRefusedNotCrashed(t *testing.T) {
	db := freshTestDB(t)
	newGroupWithMemberSessions(t, db, "group-garbage", "trainer-A", "learner-A")

	for _, garbage := range []string{"not-a-uuid", "1 OR 1=1", "\x00", "../../admin"} {
		w := getUserSessionsAs(t, db, "trainer-A", []string{"member"}, "?group_id="+url.QueryEscape(garbage))

		assert.Equal(t, http.StatusForbidden, w.Code,
			"group_id=%q must be refused, never 500", garbage)
	}
}

// TestGetUserSessionsEmptyGroupFilterStaysSelfScoped pins the zero-input
// contract: an empty group_id (what a UI sends for "all groups" if it forgets to
// drop the key) must behave exactly like no group_id — self-scoped, not a silent
// widening and not an error.
func TestGetUserSessionsEmptyGroupFilterStaysSelfScoped(t *testing.T) {
	db := freshTestDB(t)
	_, sessions := newGroupWithMemberSessions(t, db, "group-empty-filter", "trainer-A", "learner-A")

	w := getUserSessionsAs(t, db, "trainer-A", []string{"member"}, "?group_id=")

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, []string{sessions["trainer-A"]}, sessionIDsIn(t, w),
		"an empty group_id must leave the listing self-scoped")
}

// --- 4. Without group_id nothing changes ------------------------------------

// TestGetUserSessionsWithoutGroupFilterStaysSelfScoped pins the regression
// guard: the default path still returns only the caller's own sessions, even for
// a teacher who manages a group full of them.
func TestGetUserSessionsWithoutGroupFilterStaysSelfScoped(t *testing.T) {
	db := freshTestDB(t)
	_, sessions := newGroupWithMemberSessions(t, db, "group-no-filter", "trainer-A", "learner-A", "learner-B")

	w := getUserSessionsAs(t, db, "trainer-A", []string{"member"}, "")

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, []string{sessions["trainer-A"]}, sessionIDsIn(t, w),
		"without group_id the endpoint must stay strictly self-scoped")
}

// --- 5. The org-context visibility rule still bounds the widening ------------

// TestGetUserSessionsGroupFilterExcludesSessionsOutsideGroupOrg pins that the
// widening inherits SupervisableByGroupOrgScope: a member's personal (NULL-org)
// session and a session launched in another org stay invisible to the teacher.
// Without this the group filter would be a wider hole than the supervision wall.
func TestGetUserSessionsGroupFilterExcludesSessionsOutsideGroupOrg(t *testing.T) {
	db := freshTestDB(t)
	group, sessions := newGroupWithMemberSessions(t, db, "group-org-scope", "trainer-A", "learner-A")

	personalSession := newOrgTerminal(t, db, "learner-A", nil)
	otherOrg := uuid.New()
	otherOrgSession := newOrgTerminal(t, db, "learner-A", &otherOrg)

	w := getUserSessionsAs(t, db, "trainer-A", []string{"member"}, "?group_id="+group.ID.String())

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	got := sessionIDsIn(t, w)
	assert.ElementsMatch(t, []string{sessions["trainer-A"], sessions["learner-A"]}, got)
	assert.NotContains(t, got, personalSession,
		"a learner's personal session must stay invisible to the teacher")
	assert.NotContains(t, got, otherOrgSession,
		"a session launched in another org must stay invisible to the teacher")
}

// --- 6. Layer 2 declaration -------------------------------------------------

// TestUserSessionsRouteDeclaresGroupScopedSelfAccess pins the Layer 2 half of the
// widening: the route can no longer be declared plain SelfScoped, because a
// SelfScoped declaration tells the enforcement middleware (and the
// /permissions/reference page) that this route never reads another user's data.
// It must declare the group-aware rule type, keyed on the group_id query param.
func TestUserSessionsRouteDeclaresGroupScopedSelfAccess(t *testing.T) {
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	terminalController.RegisterTerminalPermissions(mocks.NewMockEnforcer())

	perm, found := access.RouteRegistry.Lookup("GET", "/api/v1/terminals/user-sessions")

	require.True(t, found, "GET /terminals/user-sessions must stay declared in the RouteRegistry")
	assert.Equal(t, terminalController.GroupScopedSelf, perm.Access.Type,
		"the route widens beyond self when group_id is passed, so its declared rule type must say so")
	assert.Equal(t, "group_id", perm.Access.Param,
		"the enforcer reads the widening group id from this query parameter")
}
