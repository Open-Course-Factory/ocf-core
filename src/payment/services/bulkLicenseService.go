// src/payment/services/bulkLicenseService.go
package services

import (
	"fmt"
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
	GetBatchLicenses(batchID uuid.UUID, requestingUserID string) (*[]models.UserSubscription, error)
	GetAvailableLicenses(batchID uuid.UUID, requestingUserID string) (*[]models.UserSubscription, error)
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

	// Create batch record
	batch := &models.SubscriptionBatch{
		PurchaserUserID:          purchaserUserID,
		SubscriptionPlanID:       input.SubscriptionPlanID,
		GroupID:                  input.GroupID,
		StripeSubscriptionID:     stripeSubscriptionID,
		StripeSubscriptionItemID: stripeSubscriptionItemID,
		TotalQuantity:            input.Quantity,
		AssignedQuantity:         0,
		Status:                   "active",
		CurrentPeriodStart:       now,
		CurrentPeriodEnd:         periodEnd,
	}

	if err := s.batchRepository.Create(batch); err != nil {
		return nil, nil, fmt.Errorf("failed to create batch: %v", err)
	}

	// Create individual license records (UserSubscription)
	licenses := make([]models.UserSubscription, input.Quantity)
	for i := 0; i < input.Quantity; i++ {
		licenses[i] = models.UserSubscription{
			UserID:               "",  // Unassigned
			PurchaserUserID:      &purchaserUserID,
			SubscriptionBatchID:  &batch.ID,
			SubscriptionPlanID:   input.SubscriptionPlanID,
			StripeSubscriptionID: stripeSubscriptionID,
			StripeCustomerID:     customerID, // Use Stripe customer ID
			Status:               "unassigned",
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

	// Revoke the license
	oldUserID := license.UserID
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
