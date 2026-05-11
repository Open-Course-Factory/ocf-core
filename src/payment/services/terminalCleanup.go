// src/payment/services/terminalCleanup.go
package services

import (
	"fmt"
	organizationModels "soli/formations/src/organizations/models"
	terminalRepo "soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TerminateUserTerminals marks all active terminals as "deleted" in the database.
// This is the shared implementation used by both user and organization subscription cancellation.
//
// Semantics: "Terminate" releases the resources — a terminated subscription must not
// continue to consume quota slots. Per the SSOT consolidation in MR !218
// (models.OccupiesSlotScope counts status IN ('active','stopped')), "stopped" terminals
// still occupy a slot. To actually free the slot, we mark them "deleted" — matching
// the canonical lifecycle delete in terminalTrainerService.DeleteSession which sets
// both Status="deleted" and State="deleted". Because deleted rows are excluded by
// OccupiesSlotScope, the stored concurrent_terminals counter no longer needs explicit
// decrementing — the real-time counter (models.CountUserOccupiedSlots) reports the
// correct value directly.
//
// Note: this only updates the DB lifecycle fields — it does NOT call the tt-backend API
// to delete the actual Incus containers. Container cleanup is handled by tt-backend's own
// expiration mechanism. The DB update ensures ocf-core immediately reflects the correct
// state and prevents new terminal access via middleware checks.
func TerminateUserTerminals(db *gorm.DB, userID string) error {
	termRepository := terminalRepo.NewTerminalRepository(db)

	// Get all active terminals for this user
	terminals, err := termRepository.GetTerminalSessionsByUserID(userID, true)
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
		if terminal.Status == "active" {
			utils.Debug("Deleting terminal %s (session: %s) for user %s", terminal.ID, terminal.SessionID, userID)

			terminal.Status = "deleted"
			terminal.State = "deleted"
			if err := termRepository.UpdateTerminalSession(&terminal); err != nil {
				utils.Error("Failed to update terminal %s status for user %s: %v", terminal.SessionID, userID, err)
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
		if err := TerminateUserTerminals(db, member.UserID); err != nil {
			utils.Error("Failed to terminate terminals for org member %s (org %s): %v", member.UserID, orgID, err)
			// Continue with other members — don't let one failure block the rest
		}
	}
}
