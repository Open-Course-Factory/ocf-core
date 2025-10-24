// src/payment/services/bulkLicenseService.go
package services

import (
	"fmt"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	terminalRepo "soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BulkLicenseService interface {
	PurchaseBulkLicenses(purchaserUserID string, input dto.BulkPurchaseInput) (*models.SubscriptionBatch, *[]models.UserSubscription, error)
	AssignLicense(batchID uuid.UUID, requestingUserID string, targetUserID string) (*models.UserSubscription, error)
	RevokeLicense(licenseID uuid.UUID, requestingUserID string) error
	UpdateBatchQuantity(batchID uuid.UUID, requestingUserID string, newQuantity int) error
	GetBatchesByPurchaser(purchaserUserID string) (*[]models.SubscriptionBatch, error)
	GetBatchLicenses(batchID uuid.UUID, requestingUserID string) (*[]models.UserSubscription, error)
	GetAvailableLicenses(batchID uuid.UUID, requestingUserID string) (*[]models.UserSubscription, error)
	PermanentlyDeleteBatch(batchID uuid.UUID, requestingUserID string) error
}

type bulkLicenseService struct {
	db                  *gorm.DB
	batchRepository     repositories.SubscriptionBatchRepository
	subscriptionRepo    repositories.PaymentRepository
	planRepository      repositories.SubscriptionPlanRepository
	stripeService       StripeService
}

func NewBulkLicenseService(db *gorm.DB) BulkLicenseService {
	return &bulkLicenseService{
		db:               db,
		batchRepository:  repositories.NewSubscriptionBatchRepository(db),
		subscriptionRepo: repositories.NewPaymentRepository(db),
		planRepository:   repositories.NewSubscriptionPlanRepository(db),
		stripeService:    NewStripeService(db),
	}
}

// PurchaseBulkLicenses creates a batch purchase and individual license records
func (s *bulkLicenseService) PurchaseBulkLicenses(purchaserUserID string, input dto.BulkPurchaseInput) (*models.SubscriptionBatch, *[]models.UserSubscription, error) {
	// Get the subscription plan
	plan, err := s.planRepository.GetByID(input.SubscriptionPlanID)
	if err != nil {
		return nil, nil, fmt.Errorf("plan not found: %v", err)
	}

	if !plan.IsActive {
		return nil, nil, fmt.Errorf("this subscription plan is not active")
	}

	// Get user from Casdoor to fetch/create Stripe customer
	user, err := casdoorsdk.GetUserByUserId(purchaserUserID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user from Casdoor: %v", err)
	}

	// Get or create Stripe customer for this user
	customerID, err := s.stripeService.CreateOrGetCustomer(purchaserUserID, user.Email, user.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Stripe customer: %v", err)
	}

	utils.Debug("Creating bulk purchase for user %s with Stripe customer %s", purchaserUserID, customerID)

	// Create Stripe subscription with quantity
	stripeSub, err := s.stripeService.CreateSubscriptionWithQuantity(
		customerID, // Use Stripe customer ID, not user UUID
		plan,
		input.Quantity,
		input.PaymentMethodID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Stripe subscription: %v", err)
	}

	// Extract subscription IDs and period info from Stripe response
	stripeSubscriptionID := stripeSub.ID
	stripeSubscriptionItemID := stripeSub.Items.Data[0].ID
	now := time.Unix(stripeSub.Items.Data[0].CurrentPeriodStart, 0)
	periodEnd := time.Unix(stripeSub.Items.Data[0].CurrentPeriodEnd, 0)

	// Create batch record with pending_payment status
	// Will be activated by webhook when invoice.payment_succeeded fires
	batch := &models.SubscriptionBatch{
		PurchaserUserID:          purchaserUserID,
		SubscriptionPlanID:       input.SubscriptionPlanID,
		GroupID:                  input.GroupID,
		StripeSubscriptionID:     stripeSubscriptionID,
		StripeSubscriptionItemID: stripeSubscriptionItemID,
		TotalQuantity:            input.Quantity,
		AssignedQuantity:         0,
		Status:                   "pending_payment", // Wait for payment before activating
		CurrentPeriodStart:       now,
		CurrentPeriodEnd:         periodEnd,
	}

	if err := s.batchRepository.Create(batch); err != nil {
		return nil, nil, fmt.Errorf("failed to create batch: %v", err)
	}

	// Create individual license records with pending_payment status
	// Will be activated when payment succeeds
	licenses := make([]models.UserSubscription, input.Quantity)
	for i := 0; i < input.Quantity; i++ {
		licenses[i] = models.UserSubscription{
			UserID:               "",  // Unassigned
			PurchaserUserID:      &purchaserUserID,
			SubscriptionBatchID:  &batch.ID,
			SubscriptionPlanID:   input.SubscriptionPlanID,
			StripeSubscriptionID: stripeSubscriptionID,
			StripeCustomerID:     customerID, // Use Stripe customer ID
			Status:               "pending_payment", // Wait for payment
			CurrentPeriodStart:   now,
			CurrentPeriodEnd:     periodEnd,
		}

		if err := s.subscriptionRepo.CreateUserSubscription(&licenses[i]); err != nil {
			utils.Error("Failed to create license %d: %v", i, err)
		}
	}

	utils.Info("Bulk purchase created: %d licenses for plan %s by user %s", input.Quantity, plan.Name, purchaserUserID)

	return batch, &licenses, nil
}

// AssignLicense assigns an unassigned license to a user
func (s *bulkLicenseService) AssignLicense(batchID uuid.UUID, requestingUserID string, targetUserID string) (*models.UserSubscription, error) {
	// Get the batch
	batch, err := s.batchRepository.GetByID(batchID)
	if err != nil {
		return nil, fmt.Errorf("batch not found: %v", err)
	}

	// Verify requester is the purchaser
	if batch.PurchaserUserID != requestingUserID {
		return nil, fmt.Errorf("only the purchaser can assign licenses")
	}

	// Check if batch has available licenses
	if batch.AssignedQuantity >= batch.TotalQuantity {
		return nil, fmt.Errorf("no available licenses in this batch")
	}

	// Find an unassigned license from this batch
	var license models.UserSubscription
	err = s.db.Where("subscription_batch_id = ? AND status = ?", batchID, "unassigned").
		First(&license).Error
	if err != nil {
		return nil, fmt.Errorf("no unassigned licenses found: %v", err)
	}

	// Assign the license
	license.UserID = targetUserID
	license.Status = "active"
	license.SubscriptionType = "assigned" // Mark as assigned license for stacked subscription priority

	if err := s.subscriptionRepo.UpdateUserSubscription(&license); err != nil {
		return nil, fmt.Errorf("failed to assign license: %v", err)
	}

	// Increment assigned quantity
	if err := s.batchRepository.IncrementAssignedQuantity(batchID, 1); err != nil {
		utils.Warn("Failed to increment assigned quantity: %v", err)
	}

	utils.Info("License %s assigned to user %s from batch %s", license.ID, targetUserID, batchID)

	return &license, nil
}

// RevokeLicense removes a license assignment and returns it to the pool
func (s *bulkLicenseService) RevokeLicense(licenseID uuid.UUID, requestingUserID string) error {
	// Get the license
	license, err := s.subscriptionRepo.GetUserSubscription(licenseID)
	if err != nil {
		return fmt.Errorf("license not found: %v", err)
	}

	// Verify the license is part of a batch
	if license.SubscriptionBatchID == nil {
		return fmt.Errorf("this license is not part of a batch")
	}

	// Get the batch
	batch, err := s.batchRepository.GetByID(*license.SubscriptionBatchID)
	if err != nil {
		return fmt.Errorf("batch not found: %v", err)
	}

	// Verify requester is the purchaser
	if batch.PurchaserUserID != requestingUserID {
		return fmt.Errorf("only the purchaser can revoke licenses")
	}

	// CRITICAL: Terminate all active terminals for this user before revoking license
	oldUserID := license.UserID
	if oldUserID != "" {
		utils.Info("🔌 Terminating all active terminals for user %s due to license revocation", oldUserID)
		if err := s.terminateUserTerminals(oldUserID); err != nil {
			utils.Error("Failed to terminate terminals for user %s: %v", oldUserID, err)
			// Don't fail license revocation if terminal termination fails
		}
	}

	// Revoke the license
	license.UserID = ""
	license.Status = "unassigned"

	if err := s.subscriptionRepo.UpdateUserSubscription(license); err != nil {
		return fmt.Errorf("failed to revoke license: %v", err)
	}

	// Decrement assigned quantity
	if err := s.batchRepository.DecrementAssignedQuantity(*license.SubscriptionBatchID, 1); err != nil {
		utils.Warn("Failed to decrement assigned quantity: %v", err)
	}

	utils.Info("License %s revoked from user %s", licenseID, oldUserID)

	return nil
}

// UpdateBatchQuantity scales the batch up or down
func (s *bulkLicenseService) UpdateBatchQuantity(batchID uuid.UUID, requestingUserID string, newQuantity int) error {
	batch, err := s.batchRepository.GetByID(batchID)
	if err != nil {
		return fmt.Errorf("batch not found: %v", err)
	}

	if batch.PurchaserUserID != requestingUserID {
		return fmt.Errorf("only the purchaser can update quantity")
	}

	if newQuantity < batch.AssignedQuantity {
		return fmt.Errorf("cannot reduce quantity below assigned licenses (%d)", batch.AssignedQuantity)
	}

	oldQuantity := batch.TotalQuantity
	difference := newQuantity - oldQuantity

	if difference == 0 {
		return nil // No change
	}

	// Update Stripe subscription quantity
	_, err = s.stripeService.UpdateSubscriptionQuantity(
		batch.StripeSubscriptionID,
		batch.StripeSubscriptionItemID,
		newQuantity,
	)
	if err != nil {
		// Check if Stripe reports the subscription is already cancelled
		errMsg := err.Error()
		if containsAny(errMsg, []string{"invalid-canceled-subscription", "canceled subscription", "cancelled subscription"}) {
			utils.Warn("⚠️ Stripe reports subscription %s is cancelled - auto-cancelling batch %s", batch.StripeSubscriptionID, batchID)

			// Auto-cancel the batch and all licenses
			if cancelErr := s.autoCancelBatchFromStripeError(batchID); cancelErr != nil {
				utils.Error("Failed to auto-cancel batch: %v", cancelErr)
				return fmt.Errorf("Stripe subscription is cancelled externally and failed to sync local state: %v", cancelErr)
			}

			return fmt.Errorf("Stripe subscription was cancelled externally - batch and licenses have been cancelled locally")
		}

		return fmt.Errorf("failed to update Stripe subscription quantity: %v", err)
	}

	if difference > 0 {
		// Get Stripe customer ID from an existing license in the batch
		var existingLicense models.UserSubscription
		err = s.db.Where("subscription_batch_id = ?", batchID).First(&existingLicense).Error
		if err != nil {
			return fmt.Errorf("failed to get existing license for customer ID: %v", err)
		}

		// Adding licenses
		for i := 0; i < difference; i++ {
			license := models.UserSubscription{
				UserID:               "",
				PurchaserUserID:      &batch.PurchaserUserID,
				SubscriptionBatchID:  &batch.ID,
				SubscriptionPlanID:   batch.SubscriptionPlanID,
				StripeSubscriptionID: batch.StripeSubscriptionID,
				StripeCustomerID:     existingLicense.StripeCustomerID, // Use same customer ID as existing licenses
				Status:               "unassigned",
				CurrentPeriodStart:   batch.CurrentPeriodStart,
				CurrentPeriodEnd:     batch.CurrentPeriodEnd,
			}
			if err := s.subscriptionRepo.CreateUserSubscription(&license); err != nil {
				utils.Error("Failed to create additional license: %v", err)
			}
		}
		utils.Info("Added %d licenses to batch %s", difference, batchID)
	} else {
		// Removing licenses (only unassigned ones)
		toRemove := -difference
		var unassignedLicenses []models.UserSubscription
		s.db.Where("subscription_batch_id = ? AND status = ?", batchID, "unassigned").
			Limit(toRemove).
			Find(&unassignedLicenses)

		if len(unassignedLicenses) < toRemove {
			return fmt.Errorf("not enough unassigned licenses to remove")
		}

		for _, license := range unassignedLicenses {
			s.db.Delete(&license)
		}
		utils.Info("Removed %d licenses from batch %s", toRemove, batchID)
	}

	// Update batch total quantity
	batch.TotalQuantity = newQuantity
	if err := s.batchRepository.Update(batch); err != nil {
		return fmt.Errorf("failed to update batch: %v", err)
	}

	return nil
}

// GetBatchesByPurchaser returns all batches purchased by a user
func (s *bulkLicenseService) GetBatchesByPurchaser(purchaserUserID string) (*[]models.SubscriptionBatch, error) {
	return s.batchRepository.GetByPurchaser(purchaserUserID)
}

// GetBatchLicenses returns all licenses in a batch
func (s *bulkLicenseService) GetBatchLicenses(batchID uuid.UUID, requestingUserID string) (*[]models.UserSubscription, error) {
	batch, err := s.batchRepository.GetByID(batchID)
	if err != nil {
		return nil, fmt.Errorf("batch not found: %v", err)
	}

	if batch.PurchaserUserID != requestingUserID {
		return nil, fmt.Errorf("access denied")
	}

	var licenses []models.UserSubscription
	err = s.db.Preload("SubscriptionPlan").
		Where("subscription_batch_id = ?", batchID).
		Order("status DESC, user_id").  // Assigned first
		Find(&licenses).Error

	if err != nil {
		return nil, err
	}

	return &licenses, nil
}

// GetAvailableLicenses returns unassigned licenses in a batch
func (s *bulkLicenseService) GetAvailableLicenses(batchID uuid.UUID, requestingUserID string) (*[]models.UserSubscription, error) {
	batch, err := s.batchRepository.GetByID(batchID)
	if err != nil {
		return nil, fmt.Errorf("batch not found: %v", err)
	}

	if batch.PurchaserUserID != requestingUserID {
		return nil, fmt.Errorf("access denied")
	}

	var licenses []models.UserSubscription
	err = s.db.Preload("SubscriptionPlan").
		Where("subscription_batch_id = ? AND status = ?", batchID, "unassigned").
		Find(&licenses).Error

	if err != nil {
		return nil, err
	}

	return &licenses, nil
}

// terminateUserTerminals stops all active terminals for a user
// This uses direct repository calls to avoid circular dependency with terminalTrainer/services
func (s *bulkLicenseService) terminateUserTerminals(userID string) error {
	// Get terminal repository
	termRepository := terminalRepo.NewTerminalRepository(s.db)

	// Get all active terminals for this user
	terminals, err := termRepository.GetTerminalSessionsByUserID(userID, true)
	if err != nil {
		return fmt.Errorf("failed to get user terminals: %v", err)
	}

	if terminals == nil || len(*terminals) == 0 {
		utils.Debug("No active terminals found for user %s", userID)
		return nil
	}

	utils.Info("Found %d active terminals for user %s, terminating all", len(*terminals), userID)

	// Stop each terminal directly using repository
	terminatedCount := 0
	for _, terminal := range *terminals {
		if terminal.Status == "active" {
			utils.Debug("Stopping terminal %s (session: %s) for user %s", terminal.ID, terminal.SessionID, userID)

			// Update terminal status to stopped
			terminal.Status = "stopped"
			if err := termRepository.UpdateTerminalSession(&terminal); err != nil {
				utils.Error("Failed to update terminal %s status for user %s: %v", terminal.SessionID, userID, err)
				continue
			}

			// Decrement concurrent_terminals metric
			if err := s.decrementConcurrentTerminals(userID); err != nil {
				utils.Warn("Failed to decrement concurrent_terminals for user %s: %v", userID, err)
			}

			terminatedCount++
			utils.Debug("Successfully stopped terminal %s for user %s", terminal.SessionID, userID)
		}
	}

	utils.Info("Successfully terminated %d/%d terminals for user %s", terminatedCount, len(*terminals), userID)
	return nil
}

// decrementConcurrentTerminals decrements the concurrent_terminals metric for a user
func (s *bulkLicenseService) decrementConcurrentTerminals(userID string) error {
	// Get the user's current usage metric for concurrent_terminals
	var usageMetric models.UsageMetrics
	err := s.db.Where("user_id = ? AND metric_type = ?", userID, "concurrent_terminals").First(&usageMetric).Error
	if err != nil {
		return fmt.Errorf("usage metric not found: %v", err)
	}

	// Decrement usage by 1 (ensure it doesn't go negative)
	if usageMetric.CurrentValue > 0 {
		usageMetric.CurrentValue -= 1
		usageMetric.LastUpdated = time.Now()
		if err := s.db.Save(&usageMetric).Error; err != nil {
			return fmt.Errorf("failed to update usage metric: %v", err)
		}
	}

	return nil
}

// autoCancelBatchFromStripeError cancels a batch locally when Stripe reports it's already cancelled
// This handles the case where a subscription was cancelled in Stripe but the webhook wasn't received
func (s *bulkLicenseService) autoCancelBatchFromStripeError(batchID uuid.UUID) error {
	batch, err := s.batchRepository.GetByID(batchID)
	if err != nil {
		return fmt.Errorf("batch not found: %v", err)
	}

	utils.Info("🔄 Auto-cancelling batch %s due to external Stripe cancellation", batchID)

	// Get all licenses in this batch
	var licenses []models.UserSubscription
	err = s.db.Where("subscription_batch_id = ?", batchID).Find(&licenses).Error
	if err != nil {
		return fmt.Errorf("failed to get batch licenses: %v", err)
	}

	// Terminate terminals for all users with active assigned licenses
	now := time.Now()
	for _, license := range licenses {
		// Terminate terminals for assigned active licenses
		if license.UserID != "" && license.Status == "active" {
			utils.Info("🔌 Terminating terminals for user %s due to batch cancellation", license.UserID)
			if err := s.terminateUserTerminals(license.UserID); err != nil {
				utils.Error("Failed to terminate terminals for user %s: %v", license.UserID, err)
				// Continue with cancellation even if terminal termination fails
			}
		}

		// Cancel the license
		license.Status = "cancelled"
		license.CancelledAt = &now
		if err := s.subscriptionRepo.UpdateUserSubscription(&license); err != nil {
			utils.Error("Failed to cancel license %s: %v", license.ID, err)
		}
	}

	// Cancel the batch
	batch.Status = "cancelled"
	batch.CancelledAt = &now
	if err := s.batchRepository.Update(batch); err != nil {
		return fmt.Errorf("failed to cancel batch: %v", err)
	}

	utils.Info("✅ Auto-cancelled batch %s and %d licenses", batchID, len(licenses))
	return nil
}

// containsAny checks if a string contains any of the substrings
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

// PermanentlyDeleteBatch permanently deletes a batch and all its licenses
// This will:
// 1. Verify the requesting user is the purchaser
// 2. Cancel the Stripe subscription
// 3. Terminate all active terminals for users with assigned licenses
// 4. Delete all licenses in the batch
// 5. Delete the batch record
func (s *bulkLicenseService) PermanentlyDeleteBatch(batchID uuid.UUID, requestingUserID string) error {
	// Get the batch
	batch, err := s.batchRepository.GetByID(batchID)
	if err != nil {
		return fmt.Errorf("batch not found: %v", err)
	}

	// Verify requester is the purchaser
	if batch.PurchaserUserID != requestingUserID {
		return fmt.Errorf("only the purchaser can delete this batch")
	}

	utils.Info("🗑️ Permanently deleting batch %s with %d licenses", batchID, batch.TotalQuantity)

	// Step 1: Get all licenses in this batch to terminate terminals for assigned users
	var licenses []models.UserSubscription
	err = s.db.Where("subscription_batch_id = ?", batchID).Find(&licenses).Error
	if err != nil {
		return fmt.Errorf("failed to get batch licenses: %v", err)
	}

	// Step 2: Terminate terminals for all users with assigned licenses
	affectedUsers := make(map[string]bool)
	for _, license := range licenses {
		if license.UserID != "" && license.Status == "active" {
			affectedUsers[license.UserID] = true
		}
	}

	for userID := range affectedUsers {
		utils.Info("🔌 Terminating terminals for user %s before batch deletion", userID)
		if err := s.terminateUserTerminals(userID); err != nil {
			utils.Error("Failed to terminate terminals for user %s: %v", userID, err)
			// Continue with deletion even if terminal termination fails
		}
	}

	// Step 3: Cancel Stripe subscription if it exists
	if batch.StripeSubscriptionID != "" {
		utils.Info("💳 Cancelling Stripe subscription %s", batch.StripeSubscriptionID)
		if err := s.stripeService.CancelSubscription(batch.StripeSubscriptionID, true); err != nil {
			utils.Warn("Failed to cancel Stripe subscription %s: %v (continuing with deletion)", batch.StripeSubscriptionID, err)
			// Continue with deletion even if Stripe cancellation fails
		}
	}

	// Step 4: Delete all licenses in this batch
	utils.Info("🗑️ Deleting %d licenses from batch %s", len(licenses), batchID)
	for _, license := range licenses {
		if err := s.db.Unscoped().Delete(&license).Error; err != nil {
			utils.Error("Failed to delete license %s: %v", license.ID, err)
			// Continue with other deletions
		}
	}

	// Step 5: Delete the batch itself
	utils.Info("🗑️ Deleting batch record %s", batchID)
	if err := s.db.Unscoped().Delete(&models.SubscriptionBatch{}, batchID).Error; err != nil {
		return fmt.Errorf("failed to delete batch: %v", err)
	}

	utils.Info("✅ Successfully deleted batch %s and all %d licenses", batchID, len(licenses))
	return nil
}
