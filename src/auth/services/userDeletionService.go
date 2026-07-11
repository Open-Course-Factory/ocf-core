package services

import (
	"errors"
	"fmt"

	authModels "soli/formations/src/auth/models"
	auditModels "soli/formations/src/audit/models"
	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	paymentServices "soli/formations/src/payment/services"
	scenarioModels "soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Sentinel errors for account deletion
var (
	ErrOwnsOrganizations = errors.New("user owns non-personal organizations; transfer ownership first")
	ErrOwnsGroups        = errors.New("user owns groups; transfer ownership first")
	ErrDeletionFailed    = errors.New("account deletion failed")
)

// UserDeletionService handles RGPD right-to-erasure account deletion
type UserDeletionService interface {
	DeleteMyAccount(userID string) error
}

type userDeletionService struct {
	db          *gorm.DB
	userService UserService
}

// NewUserDeletionService creates a new UserDeletionService.
//
// userSvc is the canonical user-deletion service (Stripe cancel →
// pseudonymize → Casdoor delete → RBAC removal). DeleteMyAccount composes it
// rather than re-implementing the identity/billing cascade, so there is a
// single source of truth for that security-critical ordering.
func NewUserDeletionService(db *gorm.DB, userSvc UserService) UserDeletionService {
	return &userDeletionService{db: db, userService: userSvc}
}

// DeleteMyAccount performs the self-service RGPD right-to-erasure flow for the
// authenticated user.
//
// Ordering is load-bearing:
//  1. Pre-flight 409 gates: refuse if the user still owns a non-personal org or
//     a group. No mutation happens before these pass.
//  2. Delegate to the canonical userService.DeleteUser FIRST. That cancels every
//     active Stripe subscription (ABORTING on failure), pseudonymizes billing
//     PII, deletes the Casdoor identity, and removes RBAC policies. Delegating
//     first means a Stripe-cancel failure aborts the whole flow with ZERO
//     OCF-side mutation, so the user can safely retry.
//  3. OCF-side cascade: tear down terminals, delete scenario sessions, remove
//     memberships, delete the personal org, anonymize authorship and audit logs,
//     and delete auth tokens / settings / SSH keys.
func (s *userDeletionService) DeleteMyAccount(userID string) error {
	if err := s.assertNoOwnedOrgsOrGroups(userID); err != nil {
		return err
	}

	// Delegate identity + billing teardown BEFORE any OCF-side mutation so a
	// Stripe-cancel failure is fully retryable.
	if err := s.userService.DeleteUser(userID); err != nil {
		return err
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		return s.cascadeOCFData(tx, userID)
	})
}

// assertNoOwnedOrgsOrGroups blocks deletion while the user still owns shared
// resources others depend on (a non-personal org or a group). The personal org
// is excluded — it is deleted as part of the cascade.
func (s *userDeletionService) assertNoOwnedOrgsOrGroups(userID string) error {
	var ownedOrgCount int64
	if err := s.db.Model(&organizationModels.Organization{}).
		Where("owner_user_id = ? AND organization_type != ?", userID, organizationModels.OrgTypePersonal).
		Count(&ownedOrgCount).Error; err != nil {
		return fmt.Errorf("%w: failed to check org ownership: %v", ErrDeletionFailed, err)
	}
	if ownedOrgCount > 0 {
		return ErrOwnsOrganizations
	}

	var ownedGroupCount int64
	if err := s.db.Model(&groupModels.ClassGroup{}).
		Where("owner_user_id = ?", userID).
		Count(&ownedGroupCount).Error; err != nil {
		return fmt.Errorf("%w: failed to check group ownership: %v", ErrDeletionFailed, err)
	}
	if ownedGroupCount > 0 {
		return ErrOwnsGroups
	}

	return nil
}

// cascadeOCFData removes/anonymizes all OCF-side data for the user inside the
// caller's transaction. Each step is a named helper so the sequence reads as a
// checklist rather than one long block.
func (s *userDeletionService) cascadeOCFData(tx *gorm.DB, userID string) error {
	if err := s.terminateTerminals(tx, userID); err != nil {
		return err
	}
	if err := s.deleteUserTerminalKeys(tx, userID); err != nil {
		return err
	}
	if err := s.deleteScenarioSessions(tx, userID); err != nil {
		return err
	}
	if err := s.removeMemberships(tx, userID); err != nil {
		return err
	}
	if err := s.deletePersonalOrganization(tx, userID); err != nil {
		return err
	}
	if err := s.anonymizeScenarioAuthorship(tx, userID); err != nil {
		return err
	}
	if err := s.anonymizeAuditLogs(tx, userID); err != nil {
		return err
	}
	if err := s.deleteAuthTokens(tx, userID); err != nil {
		return err
	}
	if err := s.deleteUserSettings(tx, userID); err != nil {
		return err
	}
	s.deleteSSHKeys(tx, userID)

	// Runs LAST: the steps above filter terminal rows by user_id, so the raw id
	// must survive until they have all run. This empties it on the retained
	// (State=StateDeleted) audit rows so no terminal row links to the erased user.
	if err := s.anonymizeTerminalUserID(tx, userID); err != nil {
		return err
	}
	return nil
}

// terminateTerminals performs the real terminal teardown (State -> StateDeleted).
// It first runs the canonical payment-side helper (so the running-terminal path
// stays identical to subscription cancellation), then releases EVERY remaining
// non-deleted terminal — stopped/hibernating sessions still occupy a slot and
// disk, so erasure must free them too. TerminateUserTerminals is shared with the
// license/Stripe cancel paths and intentionally only releases running terminals;
// widening it there would wipe stopped sessions on a mere sub-cancel, so the
// extra scope is applied here in the erasure cascade only. Runs on the cascade
// tx so it is rolled back atomically if a later step fails.
//
// End state is StateDeleted, NOT StateRevoked: erasure is not revocation. The
// shared helper now marks running terminals StateRevoked (billing-lapse label),
// but an RGPD erasure removes the account entirely, so the second pass below
// (state <> StateDeleted) deliberately normalizes those just-revoked rows — and
// every other non-deleted row — back to the generic terminal-delete state. No
// "revoked" copy should survive an account that no longer exists.
func (s *userDeletionService) terminateTerminals(tx *gorm.DB, userID string) error {
	if err := paymentServices.TerminateUserTerminals(tx, userID, nil); err != nil {
		return fmt.Errorf("failed to terminate terminals: %w", err)
	}
	if err := tx.Model(&terminalModels.Terminal{}).
		Where("user_id = ? AND state <> ?", userID, terminalModels.StateDeleted).
		Update("state", terminalModels.StateDeleted).Error; err != nil {
		return fmt.Errorf("failed to release non-running terminals: %w", err)
	}
	return nil
}

// deleteUserTerminalKeys deletes the user's terminal API keys. Each
// UserTerminalKey row holds a live tt-backend credential (APIKey) keyed by
// UserID; leaving it behind would keep a usable credential for an erased user.
func (s *userDeletionService) deleteUserTerminalKeys(tx *gorm.DB, userID string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&terminalModels.UserTerminalKey{}).Error; err != nil {
		return fmt.Errorf("failed to delete user terminal keys: %w", err)
	}
	return nil
}

// anonymizeTerminalUserID empties user_id on the user's retained terminal rows.
// The rows are kept (State=StateDeleted) for audit, but must not retain a link
// to the erased identity — matching the empty-string convention used for
// scenario authorship. Must run after the user_id-filtered terminal steps.
func (s *userDeletionService) anonymizeTerminalUserID(tx *gorm.DB, userID string) error {
	if err := tx.Model(&terminalModels.Terminal{}).
		Where("user_id = ?", userID).
		Update("user_id", "").Error; err != nil {
		return fmt.Errorf("failed to anonymize terminal user id: %w", err)
	}
	return nil
}

// deleteScenarioSessions deletes the user's scenario sessions together with
// their step-progress and flag rows. Children are removed explicitly rather
// than relying on the DB-level OnDelete:CASCADE, because SQLite only enforces
// foreign keys when `PRAGMA foreign_keys = ON` is active on the executing
// connection — doing it in code keeps the behavior identical across dialects.
func (s *userDeletionService) deleteScenarioSessions(tx *gorm.DB, userID string) error {
	var sessionIDs []uuid.UUID
	if err := tx.Model(&scenarioModels.ScenarioSession{}).
		Where("user_id = ?", userID).
		Pluck("id", &sessionIDs).Error; err != nil {
		return fmt.Errorf("failed to list scenario sessions: %w", err)
	}
	if len(sessionIDs) == 0 {
		return nil
	}

	if err := tx.Where("session_id IN ?", sessionIDs).Delete(&scenarioModels.ScenarioStepProgress{}).Error; err != nil {
		return fmt.Errorf("failed to delete scenario step progress: %w", err)
	}
	if err := tx.Where("session_id IN ?", sessionIDs).Delete(&scenarioModels.ScenarioFlag{}).Error; err != nil {
		return fmt.Errorf("failed to delete scenario flags: %w", err)
	}
	if err := tx.Where("id IN ?", sessionIDs).Delete(&scenarioModels.ScenarioSession{}).Error; err != nil {
		return fmt.Errorf("failed to delete scenario sessions: %w", err)
	}
	return nil
}

// removeMemberships removes the user's org and group memberships.
func (s *userDeletionService) removeMemberships(tx *gorm.DB, userID string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&organizationModels.OrganizationMember{}).Error; err != nil {
		return fmt.Errorf("failed to delete organization memberships: %w", err)
	}
	if err := tx.Where("user_id = ?", userID).Delete(&groupModels.GroupMember{}).Error; err != nil {
		return fmt.Errorf("failed to delete group memberships: %w", err)
	}
	return nil
}

// deletePersonalOrganization deletes the user's personal org (CASCADE removes
// its members and subscription).
func (s *userDeletionService) deletePersonalOrganization(tx *gorm.DB, userID string) error {
	if err := tx.Where("owner_user_id = ? AND organization_type = ?", userID, organizationModels.OrgTypePersonal).
		Delete(&organizationModels.Organization{}).Error; err != nil {
		return fmt.Errorf("failed to delete personal organization: %w", err)
	}
	return nil
}

// anonymizeScenarioAuthorship empties created_by_id on scenarios and scenario
// assignments so the content survives but no longer points at the deleted user.
func (s *userDeletionService) anonymizeScenarioAuthorship(tx *gorm.DB, userID string) error {
	if err := tx.Model(&scenarioModels.Scenario{}).
		Where("created_by_id = ?", userID).
		Update("created_by_id", "").Error; err != nil {
		return fmt.Errorf("failed to anonymize scenarios: %w", err)
	}
	if err := tx.Model(&scenarioModels.ScenarioAssignment{}).
		Where("created_by_id = ?", userID).
		Update("created_by_id", "").Error; err != nil {
		return fmt.Errorf("failed to anonymize scenario assignments: %w", err)
	}
	return nil
}

// anonymizeAuditLogs clears the actor identity and PII (actor_id, actor_email,
// actor_ip) on the user's audit rows while preserving the events themselves.
// AuditLog.ActorID is a uuid-typed column written via uuid.Parse(userId); the
// Casdoor user-id IS that UUID, so the WHERE is built against the parsed UUID
// (a raw string compare silently no-ops in PostgreSQL).
func (s *userDeletionService) anonymizeAuditLogs(tx *gorm.DB, userID string) error {
	parsedUUID, err := uuid.Parse(userID)
	if err != nil {
		// Non-UUID userID can never match the uuid-typed actor_id column.
		utils.Warn("Skipping audit-log anonymization for non-UUID user %s: %v", userID, err)
		return nil
	}
	if err := tx.Model(&auditModels.AuditLog{}).
		Where("actor_id = ?", parsedUUID).
		Updates(map[string]any{"actor_id": nil, "actor_email": "", "actor_ip": ""}).Error; err != nil {
		return fmt.Errorf("failed to anonymize audit logs: %w", err)
	}
	return nil
}

// deleteAuthTokens deletes the user's blacklist, email-verification and
// password-reset tokens.
func (s *userDeletionService) deleteAuthTokens(tx *gorm.DB, userID string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&authModels.TokenBlacklist{}).Error; err != nil {
		return fmt.Errorf("failed to delete token blacklist: %w", err)
	}
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&authModels.EmailVerificationToken{}).Error; err != nil {
		return fmt.Errorf("failed to delete email verification tokens: %w", err)
	}
	if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&authModels.PasswordResetToken{}).Error; err != nil {
		return fmt.Errorf("failed to delete password reset tokens: %w", err)
	}
	return nil
}

// deleteUserSettings deletes the user's settings row.
func (s *userDeletionService) deleteUserSettings(tx *gorm.DB, userID string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&authModels.UserSettings{}).Error; err != nil {
		return fmt.Errorf("failed to delete user settings: %w", err)
	}
	return nil
}

// deleteSSHKeys deletes SSH keys owned by the user. SshKey has no UserID column
// — ownership is the entity-management BaseModel.OwnerIDs array (text[]). The
// membership test differs per dialect, so detect it. Best-effort: a failure is
// logged but does not abort the deletion.
func (s *userDeletionService) deleteSSHKeys(tx *gorm.DB, userID string) {
	var err error
	switch tx.Dialector.Name() {
	case "postgres":
		err = tx.Exec("DELETE FROM ssh_keys WHERE owner_ids && ARRAY[?]", userID).Error
	default:
		// SQLite stores text[] as a serialized string; match the user id substring.
		err = tx.Exec("DELETE FROM ssh_keys WHERE owner_ids LIKE ?", "%"+userID+"%").Error
	}
	if err != nil {
		utils.Warn("Failed to delete SSH keys for user %s: %v (continuing)", userID, err)
	}
}
