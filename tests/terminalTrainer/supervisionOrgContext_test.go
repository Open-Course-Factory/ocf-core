package terminalTrainer_tests

// RED tests for the org-context supervision visibility rule (owner product
// decision, follow-up to #425/#430).
//
// RULE (pinned): a session is supervisable by a teacher IFF
// terminals.organization_id is NON-NULL AND equals the managing class-group's
// organization_id. Personal-context sessions — terminal org NULL, or a
// DIFFERENT org than the managing group — are INVISIBLE to teachers on ALL
// supervision surfaces: the wall listing (ListGroupSupervisionSessions), the
// supervise WS authorization (HasSupervisionAccess), take-hand
// (TakeHandForSupervision, which re-runs HasSupervisionAccess), and the periodic
// re-auth (SupervisionStillAuthorized, which inherits from HasSupervisionAccess).
// A managing group whose own organization_id is NULL supervises NOTHING (safe
// default) — it cannot match any terminal, even a member's.
//
// ADMIN-BYPASS DECISION (pinned): platform administrators STILL supervise
// regardless of org, on both surfaces. The rule constrains TEACHERS ("no teacher
// can see personal sessions"); admins are platform operators, not teachers, and
// access.IsAdmin already bypasses group/authz checks everywhere in OCF —
// HasSupervisionAccess returns early on isAdmin before any group/org derivation,
// and the wall must not org-filter an admin caller. The existing admin-caller
// wall suite (supervisionWall_test.go, which seeds NULL-org groups/terminals)
// stays green precisely because of this bypass; TestSupervisionOrg_Wall_
// AdminBypassesOrg pins it explicitly.
//
// SEAM (assumed by these tests): HasSupervisionAccess / ListGroupSupervisionSessions
// already resolve the learner's group memberships and the managing group; the fix
// adds the org-equality predicate (compare terminals.organization_id against the
// managing class_groups.organization_id, both non-null). Signatures are UNCHANGED,
// so every test here compiles against the current surface and fails on ASSERTION,
// not compilation. The reference implementation is the group-command-history
// scope in terminalHistoryService.go:270-275 (`if group.OrganizationID != nil {
// WHERE organization_id = group.OrganizationID }`) — NOTE it does NOT yet enforce
// the stricter NULL-group-supervises-nothing arm, which this rule requires; see
// TestSupervisionOrg_Access_NullGroupOrg_Denied / _Wall_NullGroupOrg_Empty.

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// newOrgSupervisedSession builds an active group (owned by ownerUserID, with the
// learner as an active member) and the learner's active terminal, stamping the
// group and the terminal with the given orgs. A nil pointer leaves that
// organization_id NULL. This is the org-explicit sibling of newSupervisedSession,
// used to drive the visibility rule's null / matching / mismatched cases.
func newOrgSupervisedSession(t *testing.T, db *gorm.DB, groupName, ownerUserID, learnerUserID string, groupOrg, terminalOrg *uuid.UUID) (*groupModels.ClassGroup, string) {
	t.Helper()

	group := &groupModels.ClassGroup{
		Name:           groupName,
		DisplayName:    groupName,
		OwnerUserID:    ownerUserID,
		IsActive:       true,
		MaxMembers:     50,
		OrganizationID: groupOrg,
	}
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	createTestGroupMember(t, db, group.ID, ownerUserID, groupModels.GroupMemberRoleOwner)
	createTestGroupMember(t, db, group.ID, learnerUserID, groupModels.GroupMemberRoleMember)

	terminal, err := createTestTerminal(db, learnerUserID, "active", uuid.Nil)
	require.NoError(t, err)
	if terminalOrg != nil {
		require.NoError(t, db.Model(terminal).Update("organization_id", *terminalOrg).Error)
	}

	return group, terminal.SessionID
}

// wallLists reports whether sessionID appears in a wall listing.
func wallLists(sessions []terminalController.SupervisionSession, sessionID string) bool {
	for _, s := range sessions {
		if s.SessionID == sessionID {
			return true
		}
	}
	return false
}

// --- HasSupervisionAccess: the org-equality gate (manager caller) ------------

// TestSupervisionOrg_Access_OrgMatch_Allowed is the positive guard: a terminal
// whose org equals the managing group's (both non-null) is supervisable by the
// group's manager. Green today AND after the fix — guards against the rule
// over-denying legitimate org-context sessions.
func TestSupervisionOrg_Access_OrgMatch_Allowed(t *testing.T) {
	db := setupTestDB(t)

	org := uuid.New()
	group, sessionID := newOrgSupervisedSession(t, db, "group-A", "trainer-A", "learner-A", &org, &org)

	groupID, ok := terminalController.HasSupervisionAccess(db, "trainer-A", false, sessionID)

	assert.True(t, ok, "a terminal in the managing group's org must be supervisable")
	assert.Equal(t, group.ID.String(), groupID)
}

// TestSupervisionOrg_Access_TerminalOrgNull_Denied pins that a personal-context
// session (terminal org NULL) is NOT supervisable, even by a genuine manager of
// a group the learner belongs to. RED today: HasSupervisionAccess ignores org.
func TestSupervisionOrg_Access_TerminalOrgNull_Denied(t *testing.T) {
	db := setupTestDB(t)

	org := uuid.New()
	_, sessionID := newOrgSupervisedSession(t, db, "group-A", "trainer-A", "learner-A", &org, nil)

	groupID, ok := terminalController.HasSupervisionAccess(db, "trainer-A", false, sessionID)

	assert.False(t, ok, "a NULL-org (personal) session must be invisible to teachers")
	assert.Empty(t, groupID)
}

// TestSupervisionOrg_Access_TerminalOrgDiffers_Denied pins that a session
// launched in a DIFFERENT org than the managing group is NOT supervisable. RED
// today.
func TestSupervisionOrg_Access_TerminalOrgDiffers_Denied(t *testing.T) {
	db := setupTestDB(t)

	groupOrg := uuid.New()
	otherOrg := uuid.New()
	_, sessionID := newOrgSupervisedSession(t, db, "group-A", "trainer-A", "learner-A", &groupOrg, &otherOrg)

	groupID, ok := terminalController.HasSupervisionAccess(db, "trainer-A", false, sessionID)

	assert.False(t, ok, "a session in another org must not be supervisable via this group")
	assert.Empty(t, groupID)
}

// TestSupervisionOrg_Access_NullGroupOrg_Denied pins the safe default: a
// managing group whose OWN org is NULL supervises nothing, even a member's
// org-stamped session. RED today (and diverges from the group-command-history
// reference, which does not filter when group.OrganizationID is nil).
func TestSupervisionOrg_Access_NullGroupOrg_Denied(t *testing.T) {
	db := setupTestDB(t)

	termOrg := uuid.New()
	_, sessionID := newOrgSupervisedSession(t, db, "group-A", "trainer-A", "learner-A", nil, &termOrg)

	groupID, ok := terminalController.HasSupervisionAccess(db, "trainer-A", false, sessionID)

	assert.False(t, ok, "a NULL-org managing group must supervise nothing (safe default)")
	assert.Empty(t, groupID)
}

// TestSupervisionOrg_Access_AdminBypassesOrg pins the admin-bypass decision: a
// platform administrator supervises regardless of org — here a NULL-org
// (personal) session a teacher could never see. Green today and after the fix.
func TestSupervisionOrg_Access_AdminBypassesOrg(t *testing.T) {
	db := setupTestDB(t)

	org := uuid.New()
	_, sessionID := newOrgSupervisedSession(t, db, "group-A", "trainer-A", "learner-A", &org, nil)

	_, ok := terminalController.HasSupervisionAccess(db, "admin-user", true, sessionID)

	assert.True(t, ok, "a platform admin must bypass the org gate (ops, not a teacher)")
}

// --- Wall listing: the org-equality gate (manager caller) --------------------

// TestSupervisionOrg_Wall_OrgMatch_Listed is the positive guard: an org-matched
// member session appears on the manager's wall. Green today and after the fix.
func TestSupervisionOrg_Wall_OrgMatch_Listed(t *testing.T) {
	db := setupTestDB(t)

	org := uuid.New()
	group, sessionID := newOrgSupervisedSession(t, db, "group-A", "trainer-A", "learner-A", &org, &org)

	sessions, ok := terminalController.ListGroupSupervisionSessions(db, group.ID.String(), "trainer-A", false)
	require.True(t, ok)

	assert.True(t, wallLists(sessions, sessionID),
		"an org-matched member session must appear on the wall")
}

// TestSupervisionOrg_Wall_TerminalOrgNull_NotListed pins that a NULL-org
// (personal) member session does NOT leak onto the manager's wall. RED today:
// the wall filters only membership + expiry, not org.
func TestSupervisionOrg_Wall_TerminalOrgNull_NotListed(t *testing.T) {
	db := setupTestDB(t)

	org := uuid.New()
	group, sessionID := newOrgSupervisedSession(t, db, "group-A", "trainer-A", "learner-A", &org, nil)

	sessions, ok := terminalController.ListGroupSupervisionSessions(db, group.ID.String(), "trainer-A", false)
	require.True(t, ok)

	assert.False(t, wallLists(sessions, sessionID),
		"a NULL-org (personal) session must not appear on the supervision wall")
}

// TestSupervisionOrg_Wall_TerminalOrgDiffers_NotListed pins that a member
// session launched in another org does NOT appear on this group's wall. RED
// today.
func TestSupervisionOrg_Wall_TerminalOrgDiffers_NotListed(t *testing.T) {
	db := setupTestDB(t)

	groupOrg := uuid.New()
	otherOrg := uuid.New()
	group, sessionID := newOrgSupervisedSession(t, db, "group-A", "trainer-A", "learner-A", &groupOrg, &otherOrg)

	sessions, ok := terminalController.ListGroupSupervisionSessions(db, group.ID.String(), "trainer-A", false)
	require.True(t, ok)

	assert.False(t, wallLists(sessions, sessionID),
		"a session in another org must not appear on this group's wall")
}

// TestSupervisionOrg_Wall_NullGroupOrg_Empty pins the safe default on the wall:
// a NULL-org managing group lists NOTHING, even an org-stamped member session.
// RED today. Asserts emptiness without over-constraining the ok flag (the caller
// genuinely manages the group; only the org gate empties the result).
func TestSupervisionOrg_Wall_NullGroupOrg_Empty(t *testing.T) {
	db := setupTestDB(t)

	termOrg := uuid.New()
	group, sessionID := newOrgSupervisedSession(t, db, "group-A", "trainer-A", "learner-A", nil, &termOrg)

	sessions, _ := terminalController.ListGroupSupervisionSessions(db, group.ID.String(), "trainer-A", false)

	assert.Empty(t, sessions, "a NULL-org managing group must list no sessions (safe default)")
	assert.False(t, wallLists(sessions, sessionID))
}

// TestSupervisionOrg_Wall_AdminBypassesOrg pins the admin-bypass decision on the
// wall: an administrator sees a member session even when its org differs from the
// group's (a teacher never would). Green today and after the fix — this is the
// guard that keeps the existing admin-caller wall suite valid under the new rule.
func TestSupervisionOrg_Wall_AdminBypassesOrg(t *testing.T) {
	db := setupTestDB(t)

	groupOrg := uuid.New()
	otherOrg := uuid.New()
	group, sessionID := newOrgSupervisedSession(t, db, "group-A", "trainer-A", "learner-A", &groupOrg, &otherOrg)

	sessions, ok := terminalController.ListGroupSupervisionSessions(db, group.ID.String(), "admin-user", true)
	require.True(t, ok)

	assert.True(t, wallLists(sessions, sessionID),
		"a platform admin must see sessions regardless of org (ops, not a teacher)")
}
