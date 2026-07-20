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

	// Of those, one the caller OWNS (ClassGroup.OwnerUserID)...
	var owned groupModels.ClassGroup
	if err := db.Where("id IN ? AND owner_user_id = ?", learnerGroupIDs, callerUserID).
		First(&owned).Error; err == nil {
		return owned.ID.String(), true
	}
	// ...or one where the caller holds an active manager/owner membership role.
	var membership groupModels.GroupMember
	if err := db.Where("group_id IN ? AND user_id = ? AND is_active = ? AND role IN ?",
		learnerGroupIDs, callerUserID, true, managerRoles).
		First(&membership).Error; err == nil {
		return membership.GroupID.String(), true
	}

	return "", false
}

// callerManagesGroup reports whether callerUserID is manager+ of groupID (owner via
// ClassGroup.OwnerUserID or an active manager/owner group_members role).
func callerManagesGroup(db *gorm.DB, groupID uuid.UUID, callerUserID string) bool {
	var owned groupModels.ClassGroup
	if err := db.Where("id = ? AND owner_user_id = ?", groupID, callerUserID).First(&owned).Error; err == nil {
		return true
	}
	var membership groupModels.GroupMember
	if err := db.Where("group_id = ? AND user_id = ? AND is_active = ? AND role IN ?",
		groupID, callerUserID, true, managerRoles).First(&membership).Error; err == nil {
		return true
	}
	return false
}

// PlanAllowsSupervision reports whether the plan carries the session-supervision
// feature. The controller ANDs this with HasSupervisionAccess, so a valid manager
// on a plan without the feature is still denied.
func PlanAllowsSupervision(plan *paymentModels.SubscriptionPlan) bool {
	return plan != nil && plan.SessionSupervisionEnabled
}

// buildSupervisionAudit assembles a content-rich audit entry for a supervision
// event. Actor, target session, and managing group are recorded in Metadata (and
// mirrored onto typed fields where the identifiers are real UUIDs) so the trail
// answers "who supervised whom, via which group".
func buildSupervisionAudit(event auditModels.AuditEventType, actorUserID, sessionID, groupID string) auditModels.AuditLogCreate {
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
		Status:     "success",
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
// the interactive hand. It re-verifies authorization (long-lived WS recheck), then
// writes AuditEventSupervisionTakeHand FIRST — if the audit write fails, the
// promotion is refused (fail-closed) and the caller MUST NOT promote.
func TakeHandForSupervision(db *gorm.DB, audit auditServices.AuditService, actorUserID string, isAdmin bool, sessionID, groupID string) error {
	if _, ok := HasSupervisionAccess(db, actorUserID, isAdmin, sessionID); !ok {
		return fmt.Errorf("supervision not authorized")
	}
	if err := audit.Log(buildSupervisionAudit(auditModels.AuditEventSupervisionTakeHand, actorUserID, sessionID, groupID)); err != nil {
		return err
	}
	return nil
}

// ListGroupSupervisionSessions returns the active member terminal sessions of a
// single group, but only when the caller is manager+ of that group (or admin).
// It never leaks sessions from other groups: only sessions owned by active members
// of groupID are returned. ok=false denies a non-manager caller.
func ListGroupSupervisionSessions(db *gorm.DB, groupID, callerUserID string, isAdmin bool) (sessions []terminalModels.Terminal, ok bool) {
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
		return []terminalModels.Terminal{}, true
	}

	var out []terminalModels.Terminal
	if err := db.Where("user_id IN ? AND state = ?", memberIDs, terminalModels.StateRunning).
		Find(&out).Error; err != nil {
		return nil, false
	}
	return out, true
}
