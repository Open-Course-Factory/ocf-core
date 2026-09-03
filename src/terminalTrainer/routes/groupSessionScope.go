package terminalController

// groupSessionScope.go — the SINGLE canonical answer to "may this caller read the
// terminal sessions of that class-group's members, and which sessions exactly?"
//
// Two surfaces ask that question and MUST NOT drift:
//   - the supervision wall, GET /class-groups/:id/terminal-sessions
//     (ListGroupSupervisionSessions, supervision.go)
//   - the group filter on the learner/teacher session list,
//     GET /terminals/user-sessions?group_id=… (ListGroupMemberSessions)
//
// Both route through MayListGroupSessions for the authorization half and
// groupMemberSessions for the visibility half; only the extra narrowing differs
// (the wall keeps live tiles only). The Layer 2 rule type GroupScopedSelf below
// puts the same predicate in front of the handler, so the declarative rule and
// the handler can never disagree.

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	access "soli/formations/src/auth/access"
	entityManagementModels "soli/formations/src/entityManagement/models"
	groupModels "soli/formations/src/groups/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// GroupScopedSelf is the Layer 2 access rule type for a self-scoped listing that
// a group manager may widen to the whole group through a QUERY parameter (named
// by AccessRule.Param) — unlike the built-in GroupRole, which reads a path
// parameter. With the parameter absent the route stays self-scoped; with it
// present the caller must clear MayListGroupSessions. Declaring plain SelfScoped
// here would tell the enforcement middleware and /permissions/reference that the
// route never reads another user's data, which stopped being true with #464.
const GroupScopedSelf access.AccessRuleType = "group_scoped_self"

// managerRoles are the group_members roles that grant authority over a group's
// sessions (supervision, group-scoped listing).
var managerRoles = []groupModels.GroupMemberRole{
	groupModels.GroupMemberRoleOwner,
	groupModels.GroupMemberRoleManager,
}

// callerManagesAnyGroup returns the id of one group among candidateIDs that
// callerUserID manages — the SINGLE canonical "manager+ of this group" predicate,
// shared by HasSupervisionAccess (list of the learner's groups) and
// callerManagesGroup (a single group). Management means the group is ACTIVE and the
// caller either OWNS it (ClassGroup.OwnerUserID) or holds an active manager/owner
// group_members role. An inactive (or missing) group is never manageable.
// ok=false when none qualifies.
func callerManagesAnyGroup(db *gorm.DB, candidateIDs []uuid.UUID, callerUserID string) (groupID string, ok bool) {
	if len(candidateIDs) == 0 {
		return "", false
	}
	// Restrict to ACTIVE groups (L1): an inactive class-group grants no authority.
	// Live supervision is a grant, so an archived class is out; its past
	// sessions stay reachable through the teacher dashboard.
	var activeIDs []uuid.UUID
	if err := db.Model(&groupModels.ClassGroup{}).
		Where("id IN ?", candidateIDs).
		Scopes(entityManagementModels.NotArchived("class_groups")).
		Pluck("id", &activeIDs).Error; err != nil || len(activeIDs) == 0 {
		return "", false
	}
	// One the caller OWNS (ClassGroup.OwnerUserID)...
	var owned groupModels.ClassGroup
	if err := db.Where("id IN ? AND owner_user_id = ?", activeIDs, callerUserID).
		First(&owned).Error; err == nil {
		return owned.ID.String(), true
	}
	// ...or one where the caller holds an active manager/owner membership role.
	var membership groupModels.GroupMember
	if err := db.Where("group_id IN ? AND user_id = ? AND is_active = ? AND role IN ?",
		activeIDs, callerUserID, true, managerRoles).First(&membership).Error; err == nil {
		return membership.GroupID.String(), true
	}
	return "", false
}

// callerManagesGroup reports whether callerUserID is manager+ of groupID. It
// delegates to callerManagesAnyGroup so the management predicate lives in one place.
func callerManagesGroup(db *gorm.DB, groupID uuid.UUID, callerUserID string) bool {
	_, ok := callerManagesAnyGroup(db, []uuid.UUID{groupID}, callerUserID)
	return ok
}

// MayListGroupSessions decides whether callerUserID may read the terminal
// sessions of groupID's members, returning the parsed group id for the listing
// that follows. Platform administrators bypass; everyone else must be manager+
// of an ACTIVE group (callerManagesGroup).
//
// Fail-closed and indistinguishable: an unparseable group id, an unknown group,
// an archived one, and a group the caller merely belongs to all yield
// (uuid.Nil, false) — a caller can never tell which, so probing leaks nothing.
func MayListGroupSessions(db *gorm.DB, groupID, callerUserID string, isAdmin bool) (gid uuid.UUID, ok bool) {
	parsed, err := uuid.Parse(groupID)
	if err != nil {
		return uuid.Nil, false
	}
	if isAdmin || callerManagesGroup(db, parsed, callerUserID) {
		return parsed, true
	}
	return uuid.Nil, false
}

// groupMemberSessions returns the terminals owned by gid's ACTIVE members,
// bounded for a non-admin caller by the org-context visibility rule (single home:
// models.SupervisableByGroupOrgScope) so a teacher only ever sees sessions
// launched in the group's own organization. `narrow` applies any further scope
// the calling surface needs.
//
// This is the listing, NOT the gate: every caller must have cleared
// MayListGroupSessions first.
func groupMemberSessions(db *gorm.DB, gid uuid.UUID, isAdmin bool, narrow ...func(*gorm.DB) *gorm.DB) ([]terminalModels.Terminal, error) {
	var memberIDs []string
	if err := db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND is_active = ?", gid, true).
		Pluck("user_id", &memberIDs).Error; err != nil {
		return nil, err
	}
	// A group with no active member owns no session — return early rather than
	// let an empty IN () reach the driver.
	if len(memberIDs) == 0 {
		return []terminalModels.Terminal{}, nil
	}

	query := db.Where("user_id IN ?", memberIDs).Scopes(narrow...)
	if !isAdmin {
		// A NULL-org group lists nothing. Admins are ops, not teachers — they bypass.
		var group groupModels.ClassGroup
		if err := db.Where("id = ?", gid).First(&group).Error; err != nil {
			return nil, err
		}
		query = query.Scopes(terminalModels.SupervisableByGroupOrgScope(group.OrganizationID))
	}

	var terminals []terminalModels.Terminal
	if err := query.Find(&terminals).Error; err != nil {
		return nil, err
	}
	return terminals, nil
}

// ListGroupMemberSessions returns the terminal sessions of groupID's active
// members — running and stopped alike, matching what the self-scoped
// GET /terminals/user-sessions already shows for the caller's own sessions.
// ok=false denies the caller (see MayListGroupSessions) or reports a failed
// query, without distinguishing the two.
func ListGroupMemberSessions(db *gorm.DB, groupID, callerUserID string, isAdmin bool) (sessions []terminalModels.Terminal, ok bool) {
	gid, allowed := MayListGroupSessions(db, groupID, callerUserID, isAdmin)
	if !allowed {
		return nil, false
	}
	terminals, err := groupMemberSessions(db, gid, isAdmin)
	if err != nil {
		return nil, false
	}
	return terminals, true
}
