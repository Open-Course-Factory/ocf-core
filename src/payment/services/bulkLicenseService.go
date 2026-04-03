// src/payment/services/bulkLicenseService.go
package services

import (
	"fmt"
	"strings"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
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
	GetAccessibleBatches(userID string) (*[]models.SubscriptionBatch, error)
	GetAccessibleBatchByID(batchID uuid.UUID, userID string) (*models.SubscriptionBatch, error)
	GetBatchLicenses(batchID uuid.UUID, requestingUserID string) (*[]models.UserSubscription, error)
	GetAvailableLicenses(batchID uuid.UUID, requestingUserID string) (*[]models.UserSubscription, error)
	PermanentlyDeleteBatch(batchID uuid.UUID, requestingUserID string) error
}

type bulkLicenseService struct {
	db               *gorm.DB
	batchRepository  repositories.SubscriptionBatchRepository
	subscriptionRepo repositories.PaymentRepository
	planRepository   repositories.SubscriptionPlanRepository
	stripeService    StripeService
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

// NewBulkLicenseServiceWithDeps creates a BulkLicenseService with injectable dependencies (for testing)
func NewBulkLicenseServiceWithDeps(db *gorm.DB, stripeService StripeService) BulkLicenseService {
	return &bulkLicenseService{
		db:               db,
		batchRepository:  repositories.NewSubscriptionBatchRepository(db),
		subscriptionRepo: repositories.NewPaymentRepository(db),
		planRepository:   repositories.NewSubscriptionPlanRepository(db),
		stripeService:    stripeService,
	}
}

// PurchaseBulkLicenses creates a batch purchase and individual license records
func (s *bulkLicenseService) PurchaseBulkLicenses(purchaserUserID string, input dto.BulkPurchaseInput) (*models.SubscriptionBatch, *[]models.UserSubscription, error) {
	// Get the subscription plan
	plan, err := s.planRepository.GetByID(input.SubscriptionPlanID)
	if err != nil {
		return nil, nil, fmt.Errorf("plan not found: %w", err)
	}

	if !plan.IsActive {
		return nil, nil, fmt.Errorf("this subscription plan is not active")
	}

	// Get user from Casdoor to fetch/create Stripe customer
	user, err := casdoorsdk.GetUserByUserId(purchaserUserID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user from Casdoor: %w", err)
	}

	// Get or create Stripe customer for this user
	customerID, err := s.stripeService.CreateOrGetCustomer(purchaserUserID, user.Email, user.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Stripe customer: %w", err)
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
		return nil, nil, fmt.Errorf("failed to create Stripe subscription: %w", err)
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

	// Wrap batch and license creation in a transaction to prevent partial records
	// if any license creation fails.
	var licenses []models.UserSubscription
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(batch).Error; err != nil {
			return fmt.Errorf("failed to create batch: %w", err)
		}

		// Create licenses AFTER batch so batch.ID is populated by GORM's BeforeCreate hook
		licenses = make([]models.UserSubscription, input.Quantity)
		for i := 0; i < input.Quantity; i++ {
			licenses[i] = models.UserSubscription{
				UserID:               "", // Unassigned
				PurchaserUserID:      &purchaserUserID,
				SubscriptionBatchID:  &batch.ID,
				SubscriptionPlanID:   input.SubscriptionPlanID,
				StripeSubscriptionID: &stripeSubscriptionID,
				StripeCustomerID:     &customerID,
				Status:               "pending_payment",
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     periodEnd,
			}
			if err := tx.Create(&licenses[i]).Error; err != nil {
				return fmt.Errorf("failed to create license %d: %w", i, err)
			}
		}

		return nil
	})

	if err != nil {
		// Transaction failed — cancel the Stripe subscription to avoid orphaned payment
		utils.Error("Failed to create batch records, cancelling Stripe subscription %s: %v", stripeSubscriptionID, err)
		if cancelErr := s.stripeService.CancelSubscription(stripeSubscriptionID, false); cancelErr != nil {
			utils.Error("Failed to cancel Stripe subscription %s after batch creation failure: %v", stripeSubscriptionID, cancelErr)
		}
		return nil, nil, fmt.Errorf("failed to create batch records: %w", err)
	}

	utils.Info("Bulk purchase created: %d licenses for plan %s by user %s", input.Quantity, plan.Name, purchaserUserID)

	return batch, &licenses, nil
}

// AssignLicense assigns an unassigned license to a user.
// Uses a database transaction with row-level locking to prevent race conditions
// where concurrent requests could exceed TotalQuantity.
func (s *bulkLicenseService) AssignLicense(batchID uuid.UUID, requestingUserID string, targetUserID string) (*models.UserSubscription, error) {
	// Get the batch (outside transaction for access check — read-only, no race risk)
	batch, err := s.batchRepository.GetByID(batchID)
	if err != nil {
		return nil, fmt.Errorf("batch not found: %w", err)
	}

	// Verify requester can access this batch (as purchaser or organization member)
	canAccess, err := s.canUserAccessBatch(batch, requestingUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify access: %w", err)
	}
	if !canAccess {
		return nil, fmt.Errorf("access denied: you can only assign licenses from your own batches or your organization's batches")
	}

	// Validate that the target user exists in Casdoor
	targetUser, userErr := s.getUserFromCasdoor(targetUserID)
	if userErr != nil {
		utils.Warn("Could not validate user %s in Casdoor: %v", targetUserID, userErr)
	} else if targetUser == nil {
		return nil, fmt.Errorf("user not found: the specified user ID does not exist")
	}

	// Wrap availability check + license assignment in a transaction with row locking
	// to prevent concurrent requests from exceeding TotalQuantity.
	// PostgreSQL: FOR UPDATE locks the batch row so concurrent transactions wait.
	// SQLite: the transaction boundary itself provides serialization.
	var assignedLicense models.UserSubscription
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Lock and re-read the batch row inside the transaction
		var lockedBatch models.SubscriptionBatch
		query := tx.Where("id = ?", batchID)
		// FOR UPDATE is PostgreSQL-specific; SQLite ignores it but serializes via its locking
		if tx.Dialector.Name() == "postgres" {
			query = query.Set("gorm:query_option", "FOR UPDATE")
		}
		if err := query.First(&lockedBatch).Error; err != nil {
			return fmt.Errorf("batch not found: %w", err)
		}

		// Check availability with the locked row — no other transaction can modify it concurrently
		if lockedBatch.AssignedQuantity >= lockedBatch.TotalQuantity {
			return fmt.Errorf("no available licenses in this batch")
		}

		// Find an unassigned license from this batch (within transaction)
		if err := tx.Where("subscription_batch_id = ? AND status = ?", batchID, "unassigned").
			First(&assignedLicense).Error; err != nil {
			return fmt.Errorf("no unassigned licenses found: %w", err)
		}

		// Assign the license
		assignedLicense.UserID = targetUserID
		assignedLicense.Status = "active"
		assignedLicense.SubscriptionType = "assigned"

		if err := tx.Save(&assignedLicense).Error; err != nil {
			return fmt.Errorf("failed to assign license: %w", err)
		}

		// Increment assigned quantity atomically within the same transaction
		if err := tx.Model(&models.SubscriptionBatch{}).
			Where("id = ?", batchID).
			UpdateColumn("assigned_quantity", gorm.Expr("assigned_quantity + ?", 1)).
			Error; err != nil {
			return fmt.Errorf("failed to increment assigned quantity: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	utils.Info("License %s assigned to user %s from batch %s", assignedLicense.ID, targetUserID, batchID)

	// Auto-add user to batch's linked group (non-blocking, outside transaction)
	if batch.GroupID != nil {
		s.autoAddUserToGroup(*batch.GroupID, batch.PurchaserUserID, targetUserID)
	}

	return &assignedLicense, nil
}

// RevokeLicense removes a license assignment and returns it to the pool
func (s *bulkLicenseService) RevokeLicense(licenseID uuid.UUID, requestingUserID string) error {
	// Get the license
	license, err := s.subscriptionRepo.GetUserSubscription(licenseID)
	if err != nil {
		return fmt.Errorf("license not found: %w", err)
	}

	// Verify the license is part of a batch
	if license.SubscriptionBatchID == nil {
		return fmt.Errorf("this license is not part of a batch")
	}

	// Get the batch
	batch, err := s.batchRepository.GetByID(*license.SubscriptionBatchID)
	if err != nil {
		return fmt.Errorf("batch not found: %w", err)
	}

	// Verify requester can access this batch (as purchaser or organization member)
	canAccess, err := s.canUserAccessBatch(batch, requestingUserID)
	if err != nil {
		return fmt.Errorf("failed to verify access: %w", err)
	}
	if !canAccess {
		return fmt.Errorf("access denied: you can only revoke licenses from your own batches or your organization's batches")
	}

	// CRITICAL: Terminate all active terminals for this user before revoking license
	oldUserID := license.UserID
	if oldUserID != "" {
		utils.Info("🔌 Terminating all active terminals for user %s due to license revocation", oldUserID)
		if err := TerminateUserTerminals(s.db, oldUserID); err != nil {
			utils.Error("Failed to terminate terminals for user %s: %v", oldUserID, err)
			// Don't fail license revocation if terminal termination fails
		}
	}

	// Wrap license revocation and batch quantity decrement in a transaction
	// to prevent inconsistent state if either operation fails
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Revoke the license
		license.UserID = ""
		license.Status = "unassigned"

		if err := tx.Save(license).Error; err != nil {
			return fmt.Errorf("failed to revoke license: %w", err)
		}

		// Decrement assigned quantity
		if err := tx.Model(&models.SubscriptionBatch{}).
			Where("id = ?", *license.SubscriptionBatchID).
			UpdateColumn("assigned_quantity", gorm.Expr("assigned_quantity - ?", 1)).
			Error; err != nil {
			return fmt.Errorf("failed to decrement assigned quantity: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	utils.Info("License %s revoked from user %s", licenseID, oldUserID)

	return nil
}

// UpdateBatchQuantity scales the batch up or down
func (s *bulkLicenseService) UpdateBatchQuantity(batchID uuid.UUID, requestingUserID string, newQuantity int) error {
	batch, err := s.batchRepository.GetByID(batchID)
	if err != nil {
		return fmt.Errorf("batch not found: %w", err)
	}

	// Verify requester can access this batch (as purchaser or organization member)
	canAccess, err := s.canUserAccessBatch(batch, requestingUserID)
	if err != nil {
		return fmt.Errorf("failed to verify access: %w", err)
	}
	if !canAccess {
		return fmt.Errorf("access denied: you can only update batches you purchased or your organization's batches")
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

		return fmt.Errorf("failed to update Stripe subscription quantity: %w", err)
	}

	if difference > 0 {
		// Get Stripe customer ID from an existing license in the batch
		var existingLicense models.UserSubscription
		err = s.db.Where("subscription_batch_id = ?", batchID).First(&existingLicense).Error
		if err != nil {
			return fmt.Errorf("failed to get existing license for customer ID: %w", err)
		}

		// Adding licenses
		stripeSubID := batch.StripeSubscriptionID
		for i := 0; i < difference; i++ {
			license := models.UserSubscription{
				UserID:               "",
				PurchaserUserID:      &batch.PurchaserUserID,
				SubscriptionBatchID:  &batch.ID,
				SubscriptionPlanID:   batch.SubscriptionPlanID,
				StripeSubscriptionID: &stripeSubID,
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
		return fmt.Errorf("failed to update batch: %w", err)
	}

	return nil
}

// GetBatchesByPurchaser returns all batches purchased by a user
func (s *bulkLicenseService) GetBatchesByPurchaser(purchaserUserID string) (*[]models.SubscriptionBatch, error) {
	return s.batchRepository.GetByPurchaser(purchaserUserID)
}

// GetAccessibleBatches returns all batches accessible to a user through:
// 1. Direct purchase (user is the purchaser)
// 2. Organization membership (batches purchased by other members of their team organizations)
func (s *bulkLicenseService) GetAccessibleBatches(userID string) (*[]models.SubscriptionBatch, error) {
	return s.batchRepository.GetAccessibleByUser(userID)
}

// GetAccessibleBatchByID returns a specific batch if the user can access it
// (either as the purchaser or through shared team organization membership)
func (s *bulkLicenseService) GetAccessibleBatchByID(batchID uuid.UUID, userID string) (*models.SubscriptionBatch, error) {
	batch, err := s.batchRepository.GetByID(batchID)
	if err != nil {
		return nil, fmt.Errorf("batch not found: %w", err)
	}

	canAccess, err := s.canUserAccessBatch(batch, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify access: %w", err)
	}
	if !canAccess {
		return nil, fmt.Errorf("batch not found or access denied")
	}

	return batch, nil
}

// GetBatchLicenses returns all licenses in a batch
func (s *bulkLicenseService) GetBatchLicenses(batchID uuid.UUID, requestingUserID string) (*[]models.UserSubscription, error) {
	batch, err := s.batchRepository.GetByID(batchID)
	if err != nil {
		return nil, fmt.Errorf("batch not found: %w", err)
	}

	// Verify requester can access this batch (as purchaser or organization member)
	canAccess, err := s.canUserAccessBatch(batch, requestingUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify access: %w", err)
	}
	if !canAccess {
		return nil, fmt.Errorf("access denied: you can only view licenses from your own batches or your organization's batches")
	}

	var licenses []models.UserSubscription
	err = s.db.Preload("SubscriptionPlan").
		Where("subscription_batch_id = ?", batchID).
		Order("status DESC, user_id"). // Assigned first
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
		return nil, fmt.Errorf("batch not found: %w", err)
	}

	// Verify requester can access this batch (as purchaser or organization member)
	canAccess, err := s.canUserAccessBatch(batch, requestingUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify access: %w", err)
	}
	if !canAccess {
		return nil, fmt.Errorf("access denied: you can only view available licenses from your own batches or your organization's batches")
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

// autoCancelBatchFromStripeError cancels a batch locally when Stripe reports it's already cancelled
// This handles the case where a subscription was cancelled in Stripe but the webhook wasn't received
func (s *bulkLicenseService) autoCancelBatchFromStripeError(batchID uuid.UUID) error {
	batch, err := s.batchRepository.GetByID(batchID)
	if err != nil {
		return fmt.Errorf("batch not found: %w", err)
	}

	utils.Info("🔄 Auto-cancelling batch %s due to external Stripe cancellation", batchID)

	// Get all licenses in this batch
	var licenses []models.UserSubscription
	err = s.db.Where("subscription_batch_id = ?", batchID).Find(&licenses).Error
	if err != nil {
		return fmt.Errorf("failed to get batch licenses: %w", err)
	}

	// Terminate terminals for all users with active assigned licenses
	now := time.Now()
	for _, license := range licenses {
		// Terminate terminals for assigned active licenses
		if license.UserID != "" && license.Status == "active" {
			utils.Info("🔌 Terminating terminals for user %s due to batch cancellation", license.UserID)
			if err := TerminateUserTerminals(s.db, license.UserID); err != nil {
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
		return fmt.Errorf("failed to cancel batch: %w", err)
	}

	utils.Info("✅ Auto-cancelled batch %s and %d licenses", batchID, len(licenses))
	return nil
}

// containsAny checks if a string contains any of the substrings
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if strings.Contains(s, substr) {
			return true
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
		return fmt.Errorf("batch not found: %w", err)
	}

	// Verify requester can access this batch (as purchaser or organization member)
	canAccess, err := s.canUserAccessBatch(batch, requestingUserID)
	if err != nil {
		return fmt.Errorf("failed to verify access: %w", err)
	}
	if !canAccess {
		return fmt.Errorf("access denied: you can only delete batches you purchased or your organization's batches")
	}

	utils.Info("🗑️ Permanently deleting batch %s with %d licenses", batchID, batch.TotalQuantity)

	// Step 1: Get all licenses in this batch to terminate terminals for assigned users
	var licenses []models.UserSubscription
	err = s.db.Where("subscription_batch_id = ?", batchID).Find(&licenses).Error
	if err != nil {
		return fmt.Errorf("failed to get batch licenses: %w", err)
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
		if err := TerminateUserTerminals(s.db, userID); err != nil {
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
		return fmt.Errorf("failed to delete batch: %w", err)
	}

	utils.Info("✅ Successfully deleted batch %s and all %d licenses", batchID, len(licenses))
	return nil
}

// getUserFromCasdoor safely fetches a user from Casdoor, recovering from SDK panics
// when the Casdoor client is not initialized (e.g., in unit tests)
func (s *bulkLicenseService) getUserFromCasdoor(userID string) (user *casdoorsdk.User, err error) {
	defer func() {
		if r := recover(); r != nil {
			user = nil
			err = fmt.Errorf("casdoor SDK not initialized: %v", r)
		}
	}()

	user, err = casdoorsdk.GetUserByUserId(userID)
	return
}

// canUserAccessBatch checks if a user can access a batch through:
// 1. Direct purchase (user is the purchaser)
// 2. Organization membership (user is a member of a team organization that the purchaser belongs to)
func (s *bulkLicenseService) canUserAccessBatch(batch *models.SubscriptionBatch, userID string) (bool, error) {
	// Check if user is the direct purchaser
	if batch.PurchaserUserID == userID {
		return true, nil
	}

	// Check if user and purchaser share a team organization
	var count int64
	err := s.db.Table("organization_members om1").
		Joins("JOIN organizations org ON om1.organization_id = org.id AND org.organization_type = 'team'").
		Joins("JOIN organization_members om2 ON org.id = om2.organization_id").
		Where("om1.user_id = ? AND om2.user_id = ? AND om1.is_active = true AND om2.is_active = true",
			batch.PurchaserUserID, userID).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check organization membership: %w", err)
	}

	return count > 0, nil
}

// autoAddUserToGroup adds a user to a group as a member via direct DB insert.
// Checks for existing membership to avoid duplicates.
// Failures are non-blocking: logged as warnings but never affect license assignment.
func (s *bulkLicenseService) autoAddUserToGroup(groupID uuid.UUID, purchaserUserID string, targetUserID string) {
	// Check if user is already an active member
	var count int64
	if err := s.db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND user_id = ? AND is_active = ?", groupID, targetUserID, true).
		Count(&count).Error; err != nil {
		utils.Warn("Auto-add to group: failed to check existing membership: %v", err)
		return
	}
	if count > 0 {
		return // already a member
	}

	member := &groupModels.GroupMember{
		GroupID:  groupID,
		UserID:   targetUserID,
		Role:     groupModels.GroupMemberRoleMember,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	if err := s.db.Omit("Metadata").Create(member).Error; err != nil {
		utils.Warn("Auto-add user %s to group %s failed (non-blocking): %v", targetUserID, groupID, err)
	}
}
