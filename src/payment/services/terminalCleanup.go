// src/payment/services/terminalCleanup.go
package services

import (
	"fmt"
	organizationModels "soli/formations/src/organizations/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalRepo "soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TerminateUserTerminals marks active terminals as StateDeleted in the database.
// This is the shared implementation used by both user and organization subscription cancellation.
//
// Scope (closes #314): when orgID is nil, ALL of the user's terminals are
// terminated (used for user-account cancellations: bulk-license revocation,
// batch deletion, personal Stripe sub cancel). When orgID is non-nil, only
// terminals whose organization_id matches are terminated — this is the org
// path. The org path MUST pass &orgID to avoid wiping personal-plan terminals
// of users who happen to be members of the cancelled organization (which
// caused permanent data loss after the stop→delete change).
//
// Semantics: "Terminate" releases the resources — a terminated subscription must not
// continue to consume quota. Per the SSOT consolidation in MR !218
// (models.OccupiesSlotScope counts state IN (StateRunning, StateStopped)),
// StateStopped terminals still occupy a slot. To actually free both the slot
// and the CPU/RAM budget, we mark them State=StateDeleted — matching the
// canonical lifecycle delete in terminalTrainerService.DeleteSession. Deleted
// rows are excluded by OccupiesSlotScope (the single SSOT for slot + budget
// counting), so the live counters (models.CountUserOccupiedSlots,
// QuotaService.CheckBudget) reflect the new state without any further
// bookkeeping.
//
// Note: this only updates the DB lifecycle fields — it does NOT call the tt-backend API
// to delete the actual Incus containers. Container cleanup is handled by tt-backend's own
// expiration mechanism. The DB update ensures ocf-core immediately reflects the correct
// state and prevents new terminal access via middleware checks.
func TerminateUserTerminals(db *gorm.DB, userID string, orgID *uuid.UUID) error {
	termRepository := terminalRepo.NewTerminalRepository(db)

	// Get active terminals for this user (optionally scoped to an organization)
	terminals, err := termRepository.GetTerminalSessionsByUserIDAndOrg(userID, orgID, true)
	if err != nil {
		return fmt.Errorf("failed to get user terminals: %w", err)
	}

	if terminals == nil || len(*terminals) == 0 {
		utils.Debug("No active terminals found for user %s", userID)
		return nil
	}

	utils.Info("Found %d active terminals for user %s, terminating all", len(*terminals), userID)

	terminatedCount := 0
	for _, terminal := range *terminals {
		if terminal.State == terminalModels.StateRunning {
			utils.Debug("Deleting terminal %s (session: %s) for user %s", terminal.ID, terminal.SessionID, userID)

			terminal.State = terminalModels.StateDeleted
			if err := termRepository.UpdateTerminalSession(&terminal); err != nil {
				utils.Error("Failed to update terminal %s state for user %s: %v", terminal.SessionID, userID, err)
				continue
			}

			terminatedCount++
			utils.Debug("Successfully deleted terminal %s for user %s", terminal.SessionID, userID)
		}
	}

	utils.Info("Successfully terminated %d/%d terminals for user %s", terminatedCount, len(*terminals), userID)
	return nil
}

// TerminateOrganizationMemberTerminals terminates active terminals for all members of an organization.
// Called when an organization subscription is cancelled/deleted.
// Errors are logged but do not propagate — subscription cancellation must not fail because of terminal cleanup.
func TerminateOrganizationMemberTerminals(db *gorm.DB, orgID uuid.UUID) {
	var members []organizationModels.OrganizationMember
	if err := db.Where("organization_id = ? AND is_active = ?", orgID, true).Find(&members).Error; err != nil {
		utils.Error("Failed to get organization members for org %s: %v", orgID, err)
		return
	}

	if len(members) == 0 {
		utils.Debug("No active members found for organization %s", orgID)
		return
	}

	utils.Info("Terminating terminals for %d members of organization %s due to subscription cancellation", len(members), orgID)

	for _, member := range members {
		// Scope termination to THIS org only — never touch the member's personal
		// terminals or their terminals in other orgs (closes #314).
		if err := TerminateUserTerminals(db, member.UserID, &orgID); err != nil {
			utils.Error("Failed to terminate terminals for org member %s (org %s): %v", member.UserID, orgID, err)
			// Continue with other members — don't let one failure block the rest
		}
	}
}
