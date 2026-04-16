package services

import (
	"errors"
	"fmt"

	authModels "soli/formations/src/auth/models"
	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	scenarioModels "soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"

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
	db *gorm.DB
}

// NewUserDeletionService creates a new UserDeletionService
func NewUserDeletionService(db *gorm.DB) UserDeletionService {
	return &userDeletionService{db: db}
}

// DeleteMyAccount performs a cascade deletion of all user data from the OCF database.
// It blocks if the user owns non-personal organizations or groups.
// Payment/billing records are anonymized (user_id set to "deleted") rather than deleted.
// This method does NOT handle Casdoor identity deletion or RBAC policy removal --
// those are handled by the controller after this method succeeds.
func (s *userDeletionService) DeleteMyAccount(userID string) error {
	// --- Pre-flight checks: block if user owns non-personal orgs or groups ---
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

	// --- All checks passed, run cascade in a single transaction ---
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Stop active terminal sessions
		if err := tx.Model(&terminalModels.Terminal{}).
			Where("user_id = ? AND status = ?", userID, "active").
			Update("status", "stopped").Error; err != nil {
			return fmt.Errorf("failed to stop terminals: %w", err)
		}

		// 2. Delete terminal data (terminals first, then keys due to FK)
		if err := tx.Where("user_id = ?", userID).Delete(&terminalModels.Terminal{}).Error; err != nil {
			return fmt.Errorf("failed to delete terminals: %w", err)
		}
		if err := tx.Where("user_id = ?", userID).Delete(&terminalModels.UserTerminalKey{}).Error; err != nil {
			return fmt.Errorf("failed to delete terminal keys: %w", err)
		}

		// 3. Delete scenario sessions (GORM CASCADE handles step_progress + flags)
		if err := tx.Where("user_id = ?", userID).Delete(&scenarioModels.ScenarioSession{}).Error; err != nil {
			return fmt.Errorf("failed to delete scenario sessions: %w", err)
		}

		// 4. Delete auth tokens
		if err := tx.Where("user_id = ?", userID).Delete(&authModels.TokenBlacklist{}).Error; err != nil {
			return fmt.Errorf("failed to delete token blacklist: %w", err)
		}
		if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&authModels.EmailVerificationToken{}).Error; err != nil {
			return fmt.Errorf("failed to delete email verification tokens: %w", err)
		}
		if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&authModels.PasswordResetToken{}).Error; err != nil {
			return fmt.Errorf("failed to delete password reset tokens: %w", err)
		}

		// 5. Delete personal data
		// SSH keys use OwnerIDs (pq.StringArray). In PostgreSQL: owner_ids && ARRAY[?]
		// For cross-DB compatibility, use raw SQL with dialect detection.
		dialect := tx.Dialector.Name()
		switch dialect {
		case "postgres":
			if err := tx.Exec("DELETE FROM ssh_keys WHERE owner_ids && ARRAY[?]", userID).Error; err != nil {
				utils.Warn("Failed to delete SSH keys for user %s: %v (continuing)", userID, err)
			}
		default:
			// SQLite / other: delete SSH keys where owner_ids contains the user ID
			if err := tx.Exec("DELETE FROM ssh_keys WHERE owner_ids LIKE ?", "%"+userID+"%").Error; err != nil {
				utils.Warn("Failed to delete SSH keys for user %s: %v (continuing)", userID, err)
			}
		}

		if err := tx.Where("user_id = ?", userID).Delete(&authModels.UserSettings{}).Error; err != nil {
			return fmt.Errorf("failed to delete user settings: %w", err)
		}

		// 6. Remove memberships
		if err := tx.Where("user_id = ?", userID).Delete(&organizationModels.OrganizationMember{}).Error; err != nil {
			return fmt.Errorf("failed to delete organization memberships: %w", err)
		}
		if err := tx.Where("user_id = ?", userID).Delete(&groupModels.GroupMember{}).Error; err != nil {
			return fmt.Errorf("failed to delete group memberships: %w", err)
		}

		// 7. Anonymize payment/billing records (set user_id to "deleted")
		if err := tx.Model(&paymentModels.UserSubscription{}).
			Where("user_id = ?", userID).
			Updates(map[string]any{"user_id": "deleted", "purchaser_user_id": "deleted"}).Error; err != nil {
			return fmt.Errorf("failed to anonymize user subscriptions: %w", err)
		}
		// Also anonymize subscriptions where user is purchaser but not user
		if err := tx.Model(&paymentModels.UserSubscription{}).
			Where("purchaser_user_id = ?", userID).
			Update("purchaser_user_id", "deleted").Error; err != nil {
			return fmt.Errorf("failed to anonymize purchaser subscriptions: %w", err)
		}

		if err := tx.Model(&paymentModels.BillingAddress{}).
			Where("user_id = ?", userID).
			Update("user_id", "deleted").Error; err != nil {
			return fmt.Errorf("failed to anonymize billing addresses: %w", err)
		}

		if err := tx.Model(&paymentModels.PaymentMethod{}).
			Where("user_id = ?", userID).
			Update("user_id", "deleted").Error; err != nil {
			return fmt.Errorf("failed to anonymize payment methods: %w", err)
		}

		if err := tx.Model(&paymentModels.Invoice{}).
			Where("user_id = ?", userID).
			Update("user_id", "deleted").Error; err != nil {
			return fmt.Errorf("failed to anonymize invoices: %w", err)
		}

		if err := tx.Model(&paymentModels.UsageMetrics{}).
			Where("user_id = ?", userID).
			Update("user_id", "deleted").Error; err != nil {
			return fmt.Errorf("failed to anonymize usage metrics: %w", err)
		}

		if err := tx.Model(&paymentModels.SubscriptionBatch{}).
			Where("purchaser_user_id = ?", userID).
			Update("purchaser_user_id", "deleted").Error; err != nil {
			return fmt.Errorf("failed to anonymize subscription batches: %w", err)
		}

		// Anonymize scenario authorship (nullify created_by_id)
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

		// Anonymize audit logs (set actor_id to NULL)
		if err := tx.Exec("UPDATE audit_logs SET actor_id = NULL WHERE actor_id = ?", userID).Error; err != nil {
			// Non-fatal: audit logs might use UUID format for actor_id
			utils.Warn("Failed to anonymize audit logs for user %s: %v (continuing)", userID, err)
		}

		// 8. Delete personal organization (CASCADE will remove org members + org subscription)
		if err := tx.Where("owner_user_id = ? AND organization_type = ?", userID, organizationModels.OrgTypePersonal).
			Delete(&organizationModels.Organization{}).Error; err != nil {
			return fmt.Errorf("failed to delete personal organization: %w", err)
		}

		return nil
	})
}
