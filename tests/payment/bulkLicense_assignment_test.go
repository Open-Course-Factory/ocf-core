package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAssignLicense_BatchFullReturnsError verifies that assigning when the batch
// has no remaining licenses returns an appropriate error.
func TestAssignLicense_BatchFullReturnsError(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-assignment-full"

	// Create a batch of 2 licenses, both already assigned
	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 2, 2)

	_, err := svc.AssignLicense(batch.ID, purchaserID, "extra-user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no available licenses")

	// Verify batch quantity unchanged
	var updatedBatch models.SubscriptionBatch
	require.NoError(t, db.First(&updatedBatch, batch.ID).Error)
	assert.Equal(t, 2, updatedBatch.AssignedQuantity)
	assert.Equal(t, 2, updatedBatch.TotalQuantity)
}

// TestAssignLicense_DecrementsAvailableCount verifies that each assignment
// correctly increments AssignedQuantity on the batch.
func TestAssignLicense_DecrementsAvailableCount(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-assignment-decrement"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 4, 0)

	// Assign 3 out of 4 licenses
	for i := 0; i < 3; i++ {
		targetUser := "user-dec-" + uuid.New().String()[:8]
		_, err := svc.AssignLicense(batch.ID, purchaserID, targetUser)
		require.NoError(t, err)
	}

	// Verify assigned quantity is 3
	var updatedBatch models.SubscriptionBatch
	require.NoError(t, db.First(&updatedBatch, batch.ID).Error)
	assert.Equal(t, 3, updatedBatch.AssignedQuantity)

	// Verify 1 unassigned license remains
	var unassignedCount int64
	db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ? AND status = ?", batch.ID, "unassigned").
		Count(&unassignedCount)
	assert.Equal(t, int64(1), unassignedCount)
}

// TestAssignLicense_FullFlowExhaustion creates a batch, assigns all licenses
// sequentially, then verifies the next assignment is rejected.
func TestAssignLicense_FullFlowExhaustion(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-exhaustion"
	totalLicenses := 5

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, totalLicenses, 0)

	// Assign all licenses
	assignedUsers := make([]string, totalLicenses)
	for i := 0; i < totalLicenses; i++ {
		targetUser := "exhaust-user-" + uuid.New().String()[:8]
		assignedUsers[i] = targetUser
		license, err := svc.AssignLicense(batch.ID, purchaserID, targetUser)
		require.NoError(t, err, "assignment %d should succeed", i+1)
		assert.Equal(t, targetUser, license.UserID)
		assert.Equal(t, "active", license.Status)
		assert.Equal(t, "assigned", license.SubscriptionType)
	}

	// Verify batch is fully assigned
	var updatedBatch models.SubscriptionBatch
	require.NoError(t, db.First(&updatedBatch, batch.ID).Error)
	assert.Equal(t, totalLicenses, updatedBatch.AssignedQuantity)
	assert.Equal(t, totalLicenses, updatedBatch.TotalQuantity)

	// Try to assign one more — should fail
	_, err := svc.AssignLicense(batch.ID, purchaserID, "one-too-many-user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no available licenses")

	// Verify batch quantity did not change after the failed assignment
	require.NoError(t, db.First(&updatedBatch, batch.ID).Error)
	assert.Equal(t, totalLicenses, updatedBatch.AssignedQuantity)

	// Verify no unassigned licenses remain
	var unassignedCount int64
	db.Model(&models.UserSubscription{}).
		Where("subscription_batch_id = ? AND status = ?", batch.ID, "unassigned").
		Count(&unassignedCount)
	assert.Equal(t, int64(0), unassignedCount)
}

// TestAssignLicense_TransactionAtomicity verifies that the availability check
// and license assignment happen atomically — when AssignedQuantity equals
// TotalQuantity but an unassigned license row still exists (stale data scenario),
// the transaction-protected check on the batch row prevents over-assignment.
func TestAssignLicense_TransactionAtomicity(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-atomicity"

	// Create a batch with 1 license, already marked as fully assigned in the batch row,
	// but with the license row still showing "unassigned" (simulating stale data / race)
	plan := &models.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "Atomicity Test Plan",
		PriceAmount: 500,
		Currency:    "eur",
		IsActive:    true,
	}
	require.NoError(t, db.Create(plan).Error)

	batch := &models.SubscriptionBatch{
		BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
		PurchaserUserID:      purchaserID,
		SubscriptionPlanID:   plan.ID,
		StripeSubscriptionID: "sub_atomicity_" + uuid.New().String()[:8],
		TotalQuantity:        1,
		AssignedQuantity:     1, // Already at capacity
		Status:               "active",
		CurrentPeriodStart:   time.Now().Add(-24 * time.Hour),
		CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	// Create a license row that is still "unassigned" even though batch says full
	stripeSubID := batch.StripeSubscriptionID + "-lic-stale"
	license := &models.UserSubscription{
		BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
		PurchaserUserID:      &purchaserID,
		SubscriptionBatchID:  &batch.ID,
		SubscriptionPlanID:   plan.ID,
		StripeSubscriptionID: &stripeSubID,
		Status:               "unassigned",
		CurrentPeriodStart:   batch.CurrentPeriodStart,
		CurrentPeriodEnd:     batch.CurrentPeriodEnd,
	}
	require.NoError(t, db.Create(license).Error)

	// The transaction-based check should read AssignedQuantity=1 >= TotalQuantity=1
	// and reject the assignment, even though an unassigned license row exists
	_, err := svc.AssignLicense(batch.ID, purchaserID, "sneaky-user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no available licenses")

	// Verify the license was NOT assigned
	var unchangedLicense models.UserSubscription
	require.NoError(t, db.First(&unchangedLicense, license.ID).Error)
	assert.Equal(t, "unassigned", unchangedLicense.Status)
	assert.Equal(t, "", unchangedLicense.UserID)
}
