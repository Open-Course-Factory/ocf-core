package terminalController

// supervision.go — authorization, audit, and listing for terminal supervision
// (issue #425). A trainer (group manager+) may live-watch a learner's terminal
// and take the interactive hand.
//
// SECURITY MODEL:
//   - The learner's class-group is derived SERVER-SIDE from the session record.
//     No client-supplied group id is ever trusted (IDOR guard).
//   - "manager+" means the caller OWNS the group (ClassGroup.OwnerUserID) OR holds
//     an active group_members role of manager/owner — checkGroupOwnerAccess only
//     honours ownership, which is insufficient here.
//   - Platform administrators bypass the group checks (still audited).
//   - Take-hand is audit-BEFORE-act, fail-closed: if the audit write fails, the
//     promotion is refused.

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	auditModels "soli/formations/src/audit/models"
	auditServices "soli/formations/src/audit/services"
	groupModels "soli/formations/src/groups/models"
	paymentModels "soli/formations/src/payment/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// managerRoles are the group_members roles that grant supervision authority.
var managerRoles = []groupModels.GroupMemberRole{
	groupModels.GroupMemberRoleOwner,
	groupModels.GroupMemberRoleManager,
}

// HasSupervisionAccess decides whether callerUserID may supervise the learner who
// owns sessionID, deriving the learner's group SERVER-SIDE from the session
// record. It returns the id of the group the caller manages and through which
// access is granted (needed for the audit trail).
//
// Fail-closed with no error return: an unknown/unowned session, a caller who
// manages no group the learner belongs to, or a peer member all yield ("", false)
// — never a leak of whether the session exists. isAdmin bypasses the group checks.
func HasSupervisionAccess(db *gorm.DB, callerUserID string, isAdmin bool, sessionID string) (groupID string, ok bool) {
	if isAdmin {
		return "", true
	}

	// Resolve the learner from the session record (NEVER from the request).
	var terminal terminalModels.Terminal
	if err := db.Where("session_id = ?", sessionID).First(&terminal).Error; err != nil {
		return "", false
	}
	learnerID := terminal.UserID

	// The groups the learner is an active member of.
	var learnerGroupIDs []uuid.UUID
	if err := db.Model(&groupModels.GroupMember{}).
		Where("user_id = ? AND is_active = ?", learnerID, true).
		Pluck("group_id", &learnerGroupIDs).Error; err != nil || len(learnerGroupIDs) == 0 {
		return "", false
	}

	// Of the learner's groups, one the caller manages (active group + owner or an
	// active manager/owner role) grants access — the single canonical predicate.
	return callerManagesAnyGroup(db, learnerGroupIDs, callerUserID)
}

// SupervisionStillAuthorized is the periodic re-authorization check (M1) for a
// long-lived supervise stream: it re-evaluates HasSupervisionAccess so a stream
// opened by a manager is torn down once their membership is deactivated or their
// role drops below manager. Admin stays authorized.
func SupervisionStillAuthorized(db *gorm.DB, callerUserID string, isAdmin bool, sessionID string) bool {
	_, ok := HasSupervisionAccess(db, callerUserID, isAdmin, sessionID)
	return ok
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
	var activeIDs []uuid.UUID
	if err := db.Model(&groupModels.ClassGroup{}).
		Where("id IN ? AND is_active = ?", candidateIDs, true).
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

// PlanAllowsSupervision reports whether the plan carries the session-supervision
// feature. The controller ANDs this with HasSupervisionAccess, so a valid manager
// on a plan without the feature is still denied.
func PlanAllowsSupervision(plan *paymentModels.SubscriptionPlan) bool {
	return plan != nil && plan.SessionSupervisionEnabled
}

// SessionSupportsSupervision reports whether a session is in a supervisable
// context — i.e. its owner is an active member of at least one class-group. This
// gates whether the learner's OWN console is opened with control frames on (so
// the "being watched" indicator can activate). A standalone session (owner in no
// group) stays on the default, control-free console path.
func SessionSupportsSupervision(db *gorm.DB, sessionID string) bool {
	var terminal terminalModels.Terminal
	if err := db.Where("session_id = ?", sessionID).First(&terminal).Error; err != nil {
		return false
	}
	var count int64
	if err := db.Model(&groupModels.GroupMember{}).
		Where("user_id = ? AND is_active = ?", terminal.UserID, true).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// buildSupervisionAudit assembles a content-rich audit entry for a supervision
// event. Actor, target session, and managing group are recorded in Metadata (and
// mirrored onto typed fields where the identifiers are real UUIDs) so the trail
// answers "who supervised whom, via which group".
func buildSupervisionAudit(event auditModels.AuditEventType, actorUserID, sessionID, groupID string) auditModels.AuditLogCreate {
	return buildSupervisionAuditStatus(event, actorUserID, sessionID, groupID, "success")
}

// buildSupervisionAuditStatus is buildSupervisionAudit with an explicit status,
// used to record a distinct "failed" outcome (e.g. a take-hand PATCH that could
// not be applied) without silently swallowing the error.
func buildSupervisionAuditStatus(event auditModels.AuditEventType, actorUserID, sessionID, groupID, status string) auditModels.AuditLogCreate {
	meta, _ := json.Marshal(map[string]string{
		"actor_user_id": actorUserID,
		"session_id":    sessionID,
		"group_id":      groupID,
	})
	entry := auditModels.AuditLogCreate{
		EventType:  event,
		Severity:   auditModels.AuditSeverityInfo,
		TargetType: "terminal_session",
		TargetName: sessionID,
		Action:     string(event),
		Status:     status,
		Metadata:   string(meta),
		SessionID:  sessionID,
	}
	// Actor/target ids are Casdoor user ids / session ids which are UUIDs in
	// production; set the typed fields when parseable (tests use plain strings).
	if id, err := uuid.Parse(actorUserID); err == nil {
		entry.ActorID = &id
	}
	if id, err := uuid.Parse(groupID); err == nil {
		entry.OrganizationID = &id // reuse the org column to index by managing group
	}
	return entry
}

// StartSupervision authorizes the caller (via HasSupervisionAccess) and, once
// allowed, emits AuditEventSupervisionStarted referencing the actor, session, and
// resolved group. It returns the resolved managing group id.
func StartSupervision(db *gorm.DB, audit auditServices.AuditService, actorUserID string, isAdmin bool, sessionID string) (groupID string, err error) {
	groupID, ok := HasSupervisionAccess(db, actorUserID, isAdmin, sessionID)
	if !ok {
		return "", fmt.Errorf("supervision not authorized")
	}
	if err := audit.Log(buildSupervisionAudit(auditModels.AuditEventSupervisionStarted, actorUserID, sessionID, groupID)); err != nil {
		return "", err
	}
	return groupID, nil
}

// TakeHandForSupervision performs the audit-BEFORE-act step for a trainer taking
// the interactive hand. It (1) re-verifies group authorization (long-lived WS
// recheck), (2) re-checks the plan feature — a plan revoked mid-session denies the
// promotion even when authz/group are still valid — and only then (3) writes
// AuditEventSupervisionTakeHand. If any of these fails the promotion is refused
// (fail-closed) and the caller MUST NOT promote.
func TakeHandForSupervision(db *gorm.DB, audit auditServices.AuditService, plan *paymentModels.SubscriptionPlan, actorUserID string, isAdmin bool, sessionID, groupID string) error {
	if _, ok := HasSupervisionAccess(db, actorUserID, isAdmin, sessionID); !ok {
		return fmt.Errorf("supervision not authorized")
	}
	if !PlanAllowsSupervision(plan) {
		return fmt.Errorf("plan does not permit session supervision")
	}
	if err := audit.Log(buildSupervisionAudit(auditModels.AuditEventSupervisionTakeHand, actorUserID, sessionID, groupID)); err != nil {
		return err
	}
	return nil
}

// EndSupervision bounds the supervision window in the audit trail when the observe
// WebSocket closes. If handHeld is true — the trainer disconnected while STILL
// holding the interactive hand, with no explicit release_hand frame — it first
// emits AuditEventSupervisionReleased so the trail can bound who held control and
// until when, then AuditEventSupervisionStopped. When the hand was not held it
// emits only Stopped.
func EndSupervision(db *gorm.DB, audit auditServices.AuditService, actorUserID string, isAdmin bool, sessionID, groupID string, handHeld bool) error {
	if handHeld {
		if err := audit.Log(buildSupervisionAudit(auditModels.AuditEventSupervisionReleased, actorUserID, sessionID, groupID)); err != nil {
			return err
		}
	}
	return audit.Log(buildSupervisionAudit(auditModels.AuditEventSupervisionStopped, actorUserID, sessionID, groupID))
}

// SupervisionSession is a wall tile: a live member terminal enriched with the
// learner's display identity so the trainer sees WHO owns each session, not just
// an opaque user_id. It embeds the raw Terminal (all its JSON fields are promoted)
// and adds the resolved name/email. UserName/UserEmail are empty when identity
// resolution fails — the session is listed regardless (the wall never goes blank).
type SupervisionSession struct {
	terminalModels.Terminal
	UserName  string `json:"user_name"`
	UserEmail string `json:"user_email"`
}

// ListGroupSupervisionSessions returns the live member terminal sessions of a
// single group, but only when the caller is manager+ of that group (or admin).
// It never leaks sessions from other groups: only sessions owned by active members
// of groupID are returned. ok=false denies a non-manager caller.
//
// "Live" routes through models.RunningDisplayScope (the SSOT "alive right now?"
// predicate, expiry-aware) so past-expiry zombie rows — state still 'running' but
// their tt-backend session long gone — never render as dead, unsupervisable tiles.
//
// Each session is enriched with the learner's display name/email via the swappable
// services.LookupCasdoorUserForOrgUsage seam, resolved once per unique learner id.
// A resolver error leaves the session listed with an empty name — identity is a
// display nicety, not a gate.
func ListGroupSupervisionSessions(db *gorm.DB, groupID, callerUserID string, isAdmin bool) (sessions []SupervisionSession, ok bool) {
	gid, err := uuid.Parse(groupID)
	if err != nil {
		return nil, false
	}
	if !isAdmin && !callerManagesGroup(db, gid, callerUserID) {
		return nil, false
	}

	var memberIDs []string
	if err := db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND is_active = ?", gid, true).
		Pluck("user_id", &memberIDs).Error; err != nil {
		return nil, false
	}
	if len(memberIDs) == 0 {
		return []SupervisionSession{}, true
	}

	var terminals []terminalModels.Terminal
	if err := db.Scopes(terminalModels.RunningDisplayScope).
		Where("user_id IN ?", memberIDs).
		Find(&terminals).Error; err != nil {
		return nil, false
	}

	identity := make(map[string]struct{ name, email string })
	out := make([]SupervisionSession, 0, len(terminals))
	for _, t := range terminals {
		id, seen := identity[t.UserID]
		if !seen {
			if user, err := services.LookupCasdoorUserForOrgUsage(t.UserID); err == nil && user != nil {
				id = struct{ name, email string }{user.DisplayName, user.Email}
			}
			identity[t.UserID] = id // cache even the empty fallback: don't re-hit Casdoor for a known-failing id
		}
		out = append(out, SupervisionSession{Terminal: t, UserName: id.name, UserEmail: id.email})
	}
	return out, true
}
