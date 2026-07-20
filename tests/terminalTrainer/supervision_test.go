package terminalTrainer_tests

// RED tests for terminal supervision brokering (issue #425).
//
// These tests pin the SECURITY-CRITICAL surface of the supervision feature:
// server-side group derivation (IDOR guard), role hierarchy, admin bypass,
// plan-feature gating, fail-closed auditing, and group-scoped listing. The
// WebSocket byte-plumbing (which mirrors ConnectConsole) is intentionally NOT
// exercised here — only the authorization/audit/plan-gate decisions are.
//
// The production surface these tests compile against does not exist yet.
// backend-dev implements to the signatures pinned below:
//
//   package terminalController (src/terminalTrainer/routes)
//     HasSupervisionAccess(db *gorm.DB, callerUserID string, isAdmin bool, sessionID string) (groupID string, ok bool)
//     PlanAllowsSupervision(plan *paymentModels.SubscriptionPlan) bool
//     StartSupervision(db *gorm.DB, audit auditServices.AuditService, actorUserID string, isAdmin bool, sessionID string) (groupID string, err error)
//     TakeHandForSupervision(db *gorm.DB, audit auditServices.AuditService, actorUserID string, isAdmin bool, sessionID, groupID string) error
//     ListGroupSupervisionSessions(db *gorm.DB, groupID, callerUserID string, isAdmin bool) (sessions []terminalModels.Terminal, ok bool)
//
//   package models (src/audit/models)
//     AuditEventSupervisionStarted / Stopped / TakeHand / Released
//
//   package models (src/payment/models) — SubscriptionPlan
//     SessionSupervisionEnabled bool
//
// The authz decision derives the group from the SESSION RECORD, never from a
// client-supplied group id — that is the IDOR guard the first test pins.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
	auditServices "soli/formations/src/audit/services"
	groupModels "soli/formations/src/groups/models"
	paymentModels "soli/formations/src/payment/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// --- Mock audit service ------------------------------------------------------

// mockSupervisionAudit captures Log() calls so tests can assert what was
// recorded, and can be configured to fail the write so the fail-closed
// (audit-before-act) behaviour can be exercised. It implements the full
// auditServices.AuditService interface; only Log is meaningful here.
type mockSupervisionAudit struct {
	logged []auditModels.AuditLogCreate
	logErr error
}

func (m *mockSupervisionAudit) Log(entry auditModels.AuditLogCreate) error {
	m.logged = append(m.logged, entry)
	return m.logErr
}

func (m *mockSupervisionAudit) LogAuthentication(_ *gin.Context, _ auditModels.AuditEventType, _ *uuid.UUID, _ string, _ string, _ string) {
}
func (m *mockSupervisionAudit) LogBilling(_ *gin.Context, _ auditModels.AuditEventType, _ *uuid.UUID, _ *uuid.UUID, _ string, _ *float64, _ string, _ map[string]interface{}) {
}
func (m *mockSupervisionAudit) LogOrganization(_ *gin.Context, _ auditModels.AuditEventType, _ *uuid.UUID, _ *uuid.UUID, _ *uuid.UUID, _ string, _ string, _ map[string]interface{}) {
}
func (m *mockSupervisionAudit) LogUserManagement(_ *gin.Context, _ auditModels.AuditEventType, _ *uuid.UUID, _ *uuid.UUID, _ string, _ string, _ map[string]interface{}) {
}
func (m *mockSupervisionAudit) LogSecurityEvent(_ *gin.Context, _ auditModels.AuditEventType, _ *uuid.UUID, _ *uuid.UUID, _ string, _ auditModels.AuditSeverity) {
}
func (m *mockSupervisionAudit) LogResourceAccess(_ *gin.Context, _ auditModels.AuditEventType, _ *uuid.UUID, _ *uuid.UUID, _ string, _ string) {
}
func (m *mockSupervisionAudit) GetAuditLogs(_ auditServices.AuditLogFilter) ([]auditModels.AuditLog, int64, error) {
	return nil, 0, nil
}

// entryContains reports whether the marshalled audit entry mentions the given
// substring anywhere. Used to assert actor/session/group appear without
// over-constraining backend-dev's exact field mapping.
func entryContains(t *testing.T, entry auditModels.AuditLogCreate, needle string) bool {
	t.Helper()
	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	return strings.Contains(string(raw), needle)
}

// --- Fixtures ----------------------------------------------------------------

// newSupervisedSession creates an active group owned by ownerUserID with the
// learner as an active member, plus an active terminal owned by the learner.
// Returns the group and the learner's session_id.
func newSupervisedSession(t *testing.T, db *gorm.DB, groupName, ownerUserID, learnerUserID string) (*groupModels.ClassGroup, string) {
	t.Helper()

	group := &groupModels.ClassGroup{
		Name:        groupName,
		DisplayName: groupName,
		OwnerUserID: ownerUserID,
		IsActive:    true,
		MaxMembers:  50,
	}
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	// The owner is also recorded as an owner-role membership row so the
	// derivation query (which reads group_members) treats them consistently.
	createTestGroupMember(t, db, group.ID, ownerUserID, groupModels.GroupMemberRoleOwner)
	createTestGroupMember(t, db, group.ID, learnerUserID, groupModels.GroupMemberRoleMember)

	terminal, err := createTestTerminal(db, learnerUserID, "active", uuid.Nil)
	require.NoError(t, err)

	return group, terminal.SessionID
}

// --- 1. IDOR guard: group derived from session, not request -----------------

// TestSupervision_GroupDerivedFromSession_ManagerOfLearnerGroupAllowed pins that
// a manager of the learner's own class-group is authorized, and the resolved
// groupID is that group (needed for audit).
func TestSupervision_GroupDerivedFromSession_ManagerOfLearnerGroupAllowed(t *testing.T) {
	db := setupTestDB(t)

	trainer := "trainer-A"
	learner := "learner-A"
	group, sessionID := newSupervisedSession(t, db, "group-A", trainer, learner)

	groupID, ok := terminalController.HasSupervisionAccess(db, trainer, false, sessionID)

	assert.True(t, ok, "manager of the learner's group must be authorized")
	assert.Equal(t, group.ID.String(), groupID, "resolved group must be the learner's group, derived server-side")
}

// TestSupervision_ManagerOfDifferentGroup_Denied is the IDOR test: a manager of
// group B (who does NOT manage the learner's group A) must be denied. The authz
// decision must derive the group from the session record — it must NOT accept
// any client-supplied group id. This test would FAIL if the controller trusted
// a request-provided group id, because the caller genuinely manages *some*
// group, just not the learner's.
func TestSupervision_ManagerOfDifferentGroup_Denied(t *testing.T) {
	db := setupTestDB(t)

	// Group A: trainer-A manages, learner-A is the member with the session.
	_, sessionID := newSupervisedSession(t, db, "group-A", "trainer-A", "learner-A")

	// Group B: trainer-B manages a DIFFERENT learner. trainer-B has no
	// relationship to learner-A's session.
	newSupervisedSession(t, db, "group-B", "trainer-B", "learner-B")

	groupID, ok := terminalController.HasSupervisionAccess(db, "trainer-B", false, sessionID)

	assert.False(t, ok, "a manager of a different group must NOT supervise learner-A's session")
	assert.Empty(t, groupID, "no managing group should be resolved for an unauthorized caller")
}

// TestSupervision_ManagerViaMembershipRole_Allowed pins that manager access is
// granted via a group_members role of 'manager' (not only via class-group
// ownership). checkGroupOwnerAccess only honours owner_user_id; supervision
// must also honour manager-role memberships.
func TestSupervision_ManagerViaMembershipRole_Allowed(t *testing.T) {
	db := setupTestDB(t)

	owner := "owner-A"
	manager := "manager-A"
	learner := "learner-A"
	group, sessionID := newSupervisedSession(t, db, "group-A", owner, learner)

	// Add a distinct manager-role member to the same group.
	createTestGroupMember(t, db, group.ID, manager, groupModels.GroupMemberRoleManager)

	groupID, ok := terminalController.HasSupervisionAccess(db, manager, false, sessionID)

	assert.True(t, ok, "a manager-role member must be authorized")
	assert.Equal(t, group.ID.String(), groupID)
}

// --- 2. Role hierarchy -------------------------------------------------------

// TestSupervision_PeerMember_Denied pins that a plain group member (a peer
// learner) cannot supervise another learner — supervision needs manager+.
func TestSupervision_PeerMember_Denied(t *testing.T) {
	db := setupTestDB(t)

	group, sessionID := newSupervisedSession(t, db, "group-A", "trainer-A", "learner-A")

	// peer is a plain member of the same group.
	peer := "peer-A"
	createTestGroupMember(t, db, group.ID, peer, groupModels.GroupMemberRoleMember)

	groupID, ok := terminalController.HasSupervisionAccess(db, peer, false, sessionID)

	assert.False(t, ok, "a peer member must NOT supervise another learner")
	assert.Empty(t, groupID)
}

// TestSupervision_Owner_Allowed pins that a group owner is authorized.
func TestSupervision_Owner_Allowed(t *testing.T) {
	db := setupTestDB(t)

	owner := "owner-A"
	group, sessionID := newSupervisedSession(t, db, "group-A", owner, "learner-A")

	groupID, ok := terminalController.HasSupervisionAccess(db, owner, false, sessionID)

	assert.True(t, ok, "a group owner must be authorized")
	assert.Equal(t, group.ID.String(), groupID)
}

// --- 3. Admin bypass ---------------------------------------------------------

// TestSupervision_AdminBypass_Allowed pins that a platform administrator is
// authorized regardless of group membership.
func TestSupervision_AdminBypass_Allowed(t *testing.T) {
	db := setupTestDB(t)

	_, sessionID := newSupervisedSession(t, db, "group-A", "trainer-A", "learner-A")

	// admin-user has no membership in group-A at all.
	_, ok := terminalController.HasSupervisionAccess(db, "admin-user", true, sessionID)

	assert.True(t, ok, "platform administrator must bypass group membership checks")
}

// TestSupervision_AdminBypass_StillAudited pins that admin supervision is still
// recorded (bypassing authz does not bypass the audit trail).
func TestSupervision_AdminBypass_StillAudited(t *testing.T) {
	db := setupTestDB(t)

	_, sessionID := newSupervisedSession(t, db, "group-A", "trainer-A", "learner-A")
	audit := &mockSupervisionAudit{}

	_, err := terminalController.StartSupervision(db, audit, "admin-user", true, sessionID)

	require.NoError(t, err)
	require.Len(t, audit.logged, 1, "admin supervision must still emit exactly one audit entry")
	assert.Equal(t, auditModels.AuditEventSupervisionStarted, audit.logged[0].EventType)
}

// --- 4. Plan-feature gate ----------------------------------------------------

// TestSupervision_PlanGate_AbsentDenied pins that a plan lacking the
// session_supervision feature does not permit supervision, and a plan carrying
// it does. The controller ANDs PlanAllowsSupervision with HasSupervisionAccess:
// a valid manager on a plan without the feature is still 403.
func TestSupervision_PlanGate_AbsentDenied(t *testing.T) {
	without := &paymentModels.SubscriptionPlan{Name: "Free", SessionSupervisionEnabled: false}
	with := &paymentModels.SubscriptionPlan{Name: "Pro", SessionSupervisionEnabled: true}

	assert.False(t, terminalController.PlanAllowsSupervision(without),
		"plan without session_supervision must not permit supervision")
	assert.True(t, terminalController.PlanAllowsSupervision(with),
		"plan with session_supervision must permit supervision")
}

// --- 5. Non-existent / unowned session → deny, no info leak ------------------

// TestSupervision_UnknownSession_DeniedNotError pins that an unknown session id
// resolves to (‘’, false) — a fail-closed denial, never a panic/error that
// would surface a 500 or leak whether the session exists.
func TestSupervision_UnknownSession_DeniedNotError(t *testing.T) {
	db := setupTestDB(t)

	// A valid manager exists, but the session id does not.
	newSupervisedSession(t, db, "group-A", "trainer-A", "learner-A")

	groupID, ok := terminalController.HasSupervisionAccess(db, "trainer-A", false, "does-not-exist")

	assert.False(t, ok, "unknown session must be denied")
	assert.Empty(t, groupID, "unknown session must not resolve a group")
}

// --- 6. Learner in multiple groups, caller manages only one ------------------

// TestSupervision_LearnerInMultipleGroups_ResolvesManagingGroup pins that when
// the learner belongs to several groups but the caller manages only one, access
// is granted VIA that one group, and the resolved groupID names the managing
// group (so the audit trail records the correct group).
func TestSupervision_LearnerInMultipleGroups_ResolvesManagingGroup(t *testing.T) {
	db := setupTestDB(t)

	learner := "learner-multi"
	managingTrainer := "trainer-managed"

	// Group 1: managingTrainer manages, learner is a member with the session.
	group1, sessionID := newSupervisedSession(t, db, "group-1", managingTrainer, learner)

	// Group 2: a different trainer manages; the learner is also a member here,
	// but managingTrainer has no role in group 2.
	group2 := &groupModels.ClassGroup{
		Name:        "group-2",
		DisplayName: "group-2",
		OwnerUserID: "other-trainer",
		IsActive:    true,
		MaxMembers:  50,
	}
	require.NoError(t, db.Omit("Metadata").Create(group2).Error)
	createTestGroupMember(t, db, group2.ID, learner, groupModels.GroupMemberRoleMember)

	groupID, ok := terminalController.HasSupervisionAccess(db, managingTrainer, false, sessionID)

	assert.True(t, ok, "access must be granted via the group the caller manages")
	assert.Equal(t, group1.ID.String(), groupID, "resolved group must be the managing group, not the other one")
	assert.NotEqual(t, group2.ID.String(), groupID)
}

// --- 7. Audit: content-free, fail-closed on take-hand ------------------------

// TestSupervision_StartAudit_RecordsActorSessionGroup pins that supervision
// start emits terminal.supervision.started referencing the actor, target
// session, and managing group.
func TestSupervision_StartAudit_RecordsActorSessionGroup(t *testing.T) {
	db := setupTestDB(t)

	trainer := "trainer-A"
	group, sessionID := newSupervisedSession(t, db, "group-A", trainer, "learner-A")
	audit := &mockSupervisionAudit{}

	groupID, err := terminalController.StartSupervision(db, audit, trainer, false, sessionID)

	require.NoError(t, err)
	assert.Equal(t, group.ID.String(), groupID)
	require.Len(t, audit.logged, 1)

	entry := audit.logged[0]
	assert.Equal(t, auditModels.AuditEventSupervisionStarted, entry.EventType)
	assert.True(t, entryContains(t, entry, trainer), "audit entry must reference the actor")
	assert.True(t, entryContains(t, entry, sessionID), "audit entry must reference the target session")
	assert.True(t, entryContains(t, entry, group.ID.String()), "audit entry must reference the managing group")
}

// TestSupervision_TakeHandAudit_UsesTakeHandEvent pins the take-hand event type.
func TestSupervision_TakeHandAudit_UsesTakeHandEvent(t *testing.T) {
	db := setupTestDB(t)

	trainer := "trainer-A"
	group, sessionID := newSupervisedSession(t, db, "group-A", trainer, "learner-A")
	audit := &mockSupervisionAudit{}

	err := terminalController.TakeHandForSupervision(db, audit, trainer, false, sessionID, group.ID.String())

	require.NoError(t, err)
	require.Len(t, audit.logged, 1)
	assert.Equal(t, auditModels.AuditEventSupervisionTakeHand, audit.logged[0].EventType)
}

// TestSupervision_TakeHand_FailClosedOnAuditError pins audit-before-act: if the
// audit write fails, the take-hand promotion is DENIED (error returned). The
// audit must be attempted BEFORE the act, so a failed write blocks promotion.
func TestSupervision_TakeHand_FailClosedOnAuditError(t *testing.T) {
	db := setupTestDB(t)

	trainer := "trainer-A"
	group, sessionID := newSupervisedSession(t, db, "group-A", trainer, "learner-A")
	audit := &mockSupervisionAudit{logErr: assert.AnError}

	err := terminalController.TakeHandForSupervision(db, audit, trainer, false, sessionID, group.ID.String())

	require.Error(t, err, "take-hand must fail closed when the audit write fails")
	assert.Len(t, audit.logged, 1, "the audit write must be attempted before the act (audit-before-act)")
}

// --- 8. List endpoint: group-scoped, manager-only ----------------------------

// TestSupervision_ListGroupSessions_ManagerSeesOnlyOwnGroup pins that a manager
// gets their group members' active sessions, and sessions belonging to OTHER
// groups are never returned (no cross-group leak).
func TestSupervision_ListGroupSessions_ManagerSeesOnlyOwnGroup(t *testing.T) {
	db := setupTestDB(t)

	groupA, sessionA := newSupervisedSession(t, db, "group-A", "trainer-A", "learner-A")
	_, sessionB := newSupervisedSession(t, db, "group-B", "trainer-B", "learner-B")

	sessions, ok := terminalController.ListGroupSupervisionSessions(db, groupA.ID.String(), "trainer-A", false)

	require.True(t, ok, "a manager must be able to list their group's sessions")

	got := make(map[string]bool)
	for _, s := range sessions {
		got[s.SessionID] = true
	}
	assert.True(t, got[sessionA], "group A's own member session must be listed")
	assert.False(t, got[sessionB], "group B's session must NOT leak into group A's listing")
}

// TestSupervision_ListGroupSessions_NonManagerDenied pins that a non-manager of
// the group (a plain member) cannot list the group's sessions.
func TestSupervision_ListGroupSessions_NonManagerDenied(t *testing.T) {
	db := setupTestDB(t)

	groupA, _ := newSupervisedSession(t, db, "group-A", "trainer-A", "learner-A")

	// learner-A is a plain member of group A.
	_, ok := terminalController.ListGroupSupervisionSessions(db, groupA.ID.String(), "learner-A", false)

	assert.False(t, ok, "a plain member must NOT list the group's supervision sessions")
}
