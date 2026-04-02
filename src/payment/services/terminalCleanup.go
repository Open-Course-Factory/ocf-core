// src/payment/services/terminalCleanup.go
package services

import (
	"fmt"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	terminalRepo "soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TerminateUserTerminals stops all active terminals for a given user and decrements
// the concurrent_terminals usage metric for each stopped terminal.
// This is the shared implementation used by both user and organization subscription cancellation.
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
			utils.Debug("Stopping terminal %s (session: %s) for user %s", terminal.ID, terminal.SessionID, userID)

			terminal.Status = "stopped"
			if err := termRepository.UpdateTerminalSession(&terminal); err != nil {
				utils.Error("Failed to update terminal %s status for user %s: %v", terminal.SessionID, userID, err)
				continue
			}

			// Decrement concurrent_terminals metric
			if err := decrementConcurrentTerminalsForUser(db, userID); err != nil {
				utils.Warn("Failed to decrement concurrent_terminals for user %s: %v", userID, err)
			}

			terminatedCount++
			utils.Debug("Successfully stopped terminal %s for user %s", terminal.SessionID, userID)
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

// decrementConcurrentTerminalsForUser decrements the concurrent_terminals metric for a user.
func decrementConcurrentTerminalsForUser(db *gorm.DB, userID string) error {
	var usageMetric models.UsageMetrics
	err := db.Where("user_id = ? AND metric_type = ?", userID, "concurrent_terminals").First(&usageMetric).Error
	if err != nil {
		return fmt.Errorf("usage metric not found: %w", err)
	}

	if usageMetric.CurrentValue > 0 {
		usageMetric.CurrentValue -= 1
		usageMetric.LastUpdated = time.Now()
		if err := db.Save(&usageMetric).Error; err != nil {
			return fmt.Errorf("failed to update usage metric: %w", err)
		}
	}

	return nil
}
