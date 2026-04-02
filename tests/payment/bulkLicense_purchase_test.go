// tests/payment/bulkLicense_purchase_test.go
package payment_tests

import (
	"fmt"
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestPurchaseBulkLicenses_TransactionAtomicity_HappyPath verifies that
// when all DB operations succeed, both the batch and all licenses are persisted.
func TestPurchaseBulkLicenses_TransactionAtomicity_HappyPath(t *testing.T) {
	db := freshTestDB(t)

	planID := uuid.New()
	plan := &models.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: planID},
		Name:        "Pro Plan",
		PriceAmount: 2000,
		Currency:    "eur",
		IsActive:    true,
	}
	require.NoError(t, db.Create(plan).Error)

	purchaserUserID := "purchaser-tx-happy"
	stripeSubscriptionID := "sub_test_happy"
	stripeSubscriptionItemID := "si_test_happy"
	customerID := "cus_test_happy"
	quantity := 3
	now := time.Now()
	periodEnd := now.Add(30 * 24 * time.Hour)

	batch := &models.SubscriptionBatch{
		PurchaserUserID:          purchaserUserID,
		SubscriptionPlanID:       planID,
		StripeSubscriptionID:     stripeSubscriptionID,
		StripeSubscriptionItemID: stripeSubscriptionItemID,
		TotalQuantity:            quantity,
		AssignedQuantity:         0,
		Status:                   "pending_payment",
		CurrentPeriodStart:       now,
		CurrentPeriodEnd:         periodEnd,
	}

	licenses := make([]models.UserSubscription, quantity)
	licenseStripeIDs := make([]string, quantity)
	for i := 0; i < quantity; i++ {
		licenseStripeIDs[i] = fmt.Sprintf("%s-lic-%d", stripeSubscriptionID, i)
		licenses[i] = models.UserSubscription{
			UserID:               "",
			PurchaserUserID:      &purchaserUserID,
			SubscriptionBatchID:  &batch.ID, // will be set after batch.BeforeCreate
			SubscriptionPlanID:   planID,
			StripeSubscriptionID: &licenseStripeIDs[i],
			StripeCustomerID:     &customerID,
			Status:               "pending_payment",
			CurrentPeriodStart:   now,
			CurrentPeriodEnd:     periodEnd,
		}
	}

	// Execute transaction (same pattern as PurchaseBulkLicenses)
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(batch).Error; err != nil {
			return fmt.Errorf("failed to create batch: %w", err)
		}

		// Update batch ID reference now that it's been assigned
		for i := 0; i < quantity; i++ {
			licenses[i].SubscriptionBatchID = &batch.ID
			if err := tx.Create(&licenses[i]).Error; err != nil {
				return fmt.Errorf("failed to create license %d: %w", i, err)
			}
		}

		return nil
	})

	require.NoError(t, err)

	// Verify batch was created
	var savedBatch models.SubscriptionBatch
	require.NoError(t, db.First(&savedBatch, batch.ID).Error)
	assert.Equal(t, purchaserUserID, savedBatch.PurchaserUserID)
	assert.Equal(t, quantity, savedBatch.TotalQuantity)
	assert.Equal(t, "pending_payment", savedBatch.Status)

	// Verify all licenses were created
	var savedLicenses []models.UserSubscription
	require.NoError(t, db.Where("subscription_batch_id = ?", batch.ID).Find(&savedLicenses).Error)
	assert.Len(t, savedLicenses, quantity)

	for _, lic := range savedLicenses {
		assert.Equal(t, "pending_payment", lic.Status)
		assert.Equal(t, planID, lic.SubscriptionPlanID)
	}
}

// TestPurchaseBulkLicenses_TransactionRollback_OnLicenseFailure verifies that
// if any license creation fails mid-way through the loop, the entire transaction
// is rolled back — no batch record and no partial licenses remain in the DB.
func TestPurchaseBulkLicenses_TransactionRollback_OnLicenseFailure(t *testing.T) {
	db := freshTestDB(t)

	planID := uuid.New()
	plan := &models.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: planID},
		Name:        "Pro Plan",
		PriceAmount: 2000,
		Currency:    "eur",
		IsActive:    true,
	}
	require.NoError(t, db.Create(plan).Error)

	purchaserUserID := "purchaser-tx-rollback"
	stripeSubscriptionID := "sub_test_rollback"
	stripeSubscriptionItemID := "si_test_rollback"
	customerID := "cus_test_rollback"
	quantity := 5
	failAtIndex := 2 // Simulate failure on the 3rd license (index 2)
	now := time.Now()
	periodEnd := now.Add(30 * 24 * time.Hour)

	batchID := uuid.New()
	batch := &models.SubscriptionBatch{
		BaseModel:                entityManagementModels.BaseModel{ID: batchID},
		PurchaserUserID:          purchaserUserID,
		SubscriptionPlanID:       planID,
		StripeSubscriptionID:     stripeSubscriptionID,
		StripeSubscriptionItemID: stripeSubscriptionItemID,
		TotalQuantity:            quantity,
		AssignedQuantity:         0,
		Status:                   "pending_payment",
		CurrentPeriodStart:       now,
		CurrentPeriodEnd:         periodEnd,
	}

	// Execute transaction (same pattern as PurchaseBulkLicenses) with simulated failure
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(batch).Error; err != nil {
			return fmt.Errorf("failed to create batch: %w", err)
		}

		licStripeIDs := make([]string, quantity)
		for i := 0; i < quantity; i++ {
			licStripeIDs[i] = fmt.Sprintf("%s-lic-%d", stripeSubscriptionID, i)
		}
		for i := 0; i < quantity; i++ {
			// Simulate a failure at the specified index
			if i == failAtIndex {
				return fmt.Errorf("failed to create license %d: simulated database error", i)
			}

			license := &models.UserSubscription{
				UserID:               "",
				PurchaserUserID:      &purchaserUserID,
				SubscriptionBatchID:  &batchID,
				SubscriptionPlanID:   planID,
				StripeSubscriptionID: &licStripeIDs[i],
				StripeCustomerID:     &customerID,
				Status:               "pending_payment",
				CurrentPeriodStart:   now,
				CurrentPeriodEnd:     periodEnd,
			}
			if err := tx.Create(license).Error; err != nil {
				return fmt.Errorf("failed to create license %d: %w", i, err)
			}
		}

		return nil
	})

	// Transaction should have failed
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated database error")

	// Verify NO batch was persisted (transaction rolled back)
	var batchCount int64
	db.Model(&models.SubscriptionBatch{}).Where("id = ?", batchID).Count(&batchCount)
	assert.Equal(t, int64(0), batchCount, "batch should not exist after transaction rollback")

	// Verify NO licenses were persisted (transaction rolled back)
	var licenseCount int64
	db.Model(&models.UserSubscription{}).Where("subscription_batch_id = ?", batchID).Count(&licenseCount)
	assert.Equal(t, int64(0), licenseCount, "no licenses should exist after transaction rollback")
}

// TestPurchaseBulkLicenses_TransactionRollback_OnBatchFailure verifies that
// if the batch creation itself fails, the transaction is rolled back cleanly.
func TestPurchaseBulkLicenses_TransactionRollback_OnBatchFailure(t *testing.T) {
	db := freshTestDB(t)

	planID := uuid.New()
	plan := &models.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: planID},
		Name:        "Pro Plan",
		PriceAmount: 2000,
		Currency:    "eur",
		IsActive:    true,
	}
	require.NoError(t, db.Create(plan).Error)

	// Create a batch with a duplicate StripeSubscriptionID to cause a unique constraint violation
	existingBatch := &models.SubscriptionBatch{
		BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
		PurchaserUserID:      "existing-purchaser",
		SubscriptionPlanID:   planID,
		StripeSubscriptionID: "sub_duplicate",
		TotalQuantity:        1,
		Status:               "active",
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(existingBatch).Error)

	// Try to create another batch with the same StripeSubscriptionID
	purchaserUserID := "purchaser-tx-batch-fail"
	duplicateBatch := &models.SubscriptionBatch{
		BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
		PurchaserUserID:      purchaserUserID,
		SubscriptionPlanID:   planID,
		StripeSubscriptionID: "sub_duplicate", // Same as existing — unique constraint violation
		TotalQuantity:        3,
		Status:               "pending_payment",
		CurrentPeriodStart:   time.Now(),
		CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(duplicateBatch).Error; err != nil {
			return fmt.Errorf("failed to create batch: %w", err)
		}

		// Licenses should never be reached
		t.Fatal("should not reach license creation after batch failure")
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create batch")

	// Only the original batch should exist
	var batchCount int64
	db.Model(&models.SubscriptionBatch{}).Count(&batchCount)
	assert.Equal(t, int64(1), batchCount, "only the original batch should exist")
}
