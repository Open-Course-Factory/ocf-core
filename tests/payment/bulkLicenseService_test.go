package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupBulkLicenseTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.SubscriptionPlan{},
		&models.SubscriptionBatch{},
		&models.UserSubscription{},
		&models.UsageMetrics{},
		&terminalModels.Terminal{},
		&terminalModels.UserTerminalKey{},
		&orgModels.Organization{},
		&orgModels.OrganizationMember{},
	)
	require.NoError(t, err)

	return db
}

// seedBulkLicenseTestData creates a plan, batch, and unassigned licenses for testing
func seedBulkLicenseTestData(t *testing.T, db *gorm.DB, purchaserID string, totalQty int, assignedQty int) (*models.SubscriptionPlan, *models.SubscriptionBatch, []models.UserSubscription) {
	plan := &models.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "Test Pro Plan",
		PriceAmount: 1000,
		Currency:    "eur",
		IsActive:    true,
	}
	require.NoError(t, db.Create(plan).Error)

	batch := &models.SubscriptionBatch{
		BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
		PurchaserUserID:      purchaserID,
		SubscriptionPlanID:   plan.ID,
		StripeSubscriptionID: "sub_test_" + uuid.New().String()[:8],
		TotalQuantity:        totalQty,
		AssignedQuantity:     assignedQty,
		Status:               "active",
		CurrentPeriodStart:   time.Now().Add(-24 * time.Hour),
		CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(batch).Error)

	licenses := make([]models.UserSubscription, totalQty)
	for i := 0; i < totalQty; i++ {
		// Each license needs a unique StripeSubscriptionID due to SQLite unique index
		// (PostgreSQL uses partial indexes which SQLite doesn't support)
		uniqueStripeSubID := batch.StripeSubscriptionID + "-lic-" + uuid.New().String()[:8]
		licenses[i] = models.UserSubscription{
			BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
			PurchaserUserID:      &purchaserID,
			SubscriptionBatchID:  &batch.ID,
			SubscriptionPlanID:   plan.ID,
			StripeSubscriptionID: &uniqueStripeSubID,
			Status:               "unassigned",
			CurrentPeriodStart:   batch.CurrentPeriodStart,
			CurrentPeriodEnd:     batch.CurrentPeriodEnd,
		}
		// Mark the first assignedQty licenses as assigned
		if i < assignedQty {
			userID := "assigned-user-" + uuid.New().String()[:8]
			licenses[i].UserID = userID
			licenses[i].Status = "active"
			licenses[i].SubscriptionType = "assigned"
		}
		require.NoError(t, db.Create(&licenses[i]).Error)
	}

	return plan, batch, licenses
}

// --- AssignLicense tests ---

func TestBulkLicenseService_AssignLicense_HappyPath(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-001"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 5, 0)

	targetUserID := "target-user-001"
	license, err := svc.AssignLicense(batch.ID, purchaserID, targetUserID)
	require.NoError(t, err)

	assert.Equal(t, targetUserID, license.UserID)
	assert.Equal(t, "active", license.Status)
	assert.Equal(t, "assigned", license.SubscriptionType)

	// Verify batch assigned quantity incremented
	var updatedBatch models.SubscriptionBatch
	require.NoError(t, db.First(&updatedBatch, batch.ID).Error)
	assert.Equal(t, 1, updatedBatch.AssignedQuantity)
}

func TestBulkLicenseService_AssignLicense_NoAvailableLicenses(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-002"

	// All 3 licenses are assigned
	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 3)

	_, err := svc.AssignLicense(batch.ID, purchaserID, "new-user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no available licenses")
}

func TestBulkLicenseService_AssignLicense_BatchNotFound(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	_, err := svc.AssignLicense(uuid.New(), "some-user", "target-user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "batch not found")
}

func TestBulkLicenseService_AssignLicense_AccessDenied(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-003"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 5, 0)

	// Different user (not purchaser, not in same org) tries to assign
	_, err := svc.AssignLicense(batch.ID, "unauthorized-user", "target-user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestBulkLicenseService_AssignLicense_CasdoorUnavailable(t *testing.T) {
	// When Casdoor is not initialized (unit test environment), AssignLicense
	// logs a warning but still proceeds with the assignment (graceful degradation).
	// User existence validation only blocks when Casdoor is configured and
	// returns nil user (meaning user truly doesn't exist).
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-casdoor-down"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 5, 0)

	// Should succeed despite Casdoor being unavailable (graceful degradation)
	license, err := svc.AssignLicense(batch.ID, purchaserID, "some-user-id")
	require.NoError(t, err)
	assert.Equal(t, "some-user-id", license.UserID)
	assert.Equal(t, "active", license.Status)
}

// --- RevokeLicense tests ---

func TestBulkLicenseService_RevokeLicense_HappyPath(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-004"

	// 5 licenses, 2 assigned
	_, _, licenses := seedBulkLicenseTestData(t, db, purchaserID, 5, 2)

	// Revoke the first assigned license
	assignedLicense := licenses[0]
	err := svc.RevokeLicense(assignedLicense.ID, purchaserID)
	require.NoError(t, err)

	// Verify license is now unassigned
	var updatedLicense models.UserSubscription
	require.NoError(t, db.First(&updatedLicense, assignedLicense.ID).Error)
	assert.Equal(t, "", updatedLicense.UserID)
	assert.Equal(t, "unassigned", updatedLicense.Status)
}

func TestBulkLicenseService_RevokeLicense_LicenseNotFound(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	err := svc.RevokeLicense(uuid.New(), "some-user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "license not found")
}

func TestBulkLicenseService_RevokeLicense_NotInBatch(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	// Create a standalone license (not in a batch)
	plan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Solo Plan",
		IsActive:  true,
	}
	require.NoError(t, db.Create(plan).Error)

	license := &models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             "solo-user",
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		SubscriptionType:   "personal",
		// SubscriptionBatchID is nil
	}
	require.NoError(t, db.Create(license).Error)

	err := svc.RevokeLicense(license.ID, "solo-user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not part of a batch")
}

func TestBulkLicenseService_RevokeLicense_AccessDenied(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-005"

	_, _, licenses := seedBulkLicenseTestData(t, db, purchaserID, 5, 2)

	// Unauthorized user tries to revoke
	err := svc.RevokeLicense(licenses[0].ID, "unauthorized-user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

// --- GetBatchLicenses tests ---

func TestBulkLicenseService_GetBatchLicenses_HappyPath(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-006"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 5, 2)

	licenses, err := svc.GetBatchLicenses(batch.ID, purchaserID)
	require.NoError(t, err)
	assert.Len(t, *licenses, 5)
}

func TestBulkLicenseService_GetBatchLicenses_AccessDenied(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-007"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 0)

	_, err := svc.GetBatchLicenses(batch.ID, "unauthorized-user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

// --- GetAvailableLicenses tests ---

func TestBulkLicenseService_GetAvailableLicenses_HappyPath(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-008"

	// 5 total, 2 assigned = 3 available
	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 5, 2)

	available, err := svc.GetAvailableLicenses(batch.ID, purchaserID)
	require.NoError(t, err)
	assert.Len(t, *available, 3)

	for _, lic := range *available {
		assert.Equal(t, "unassigned", lic.Status)
	}
}

// --- GetBatchesByPurchaser tests ---

func TestBulkLicenseService_GetBatchesByPurchaser(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-009"

	// Create 2 batches
	seedBulkLicenseTestData(t, db, purchaserID, 3, 0)
	seedBulkLicenseTestData(t, db, purchaserID, 5, 0)

	// Create a batch for a different user
	seedBulkLicenseTestData(t, db, "other-purchaser", 2, 0)

	batches, err := svc.GetBatchesByPurchaser(purchaserID)
	require.NoError(t, err)
	assert.Len(t, *batches, 2)
}

// --- UpdateBatchQuantity tests ---
// Note: UpdateBatchQuantity calls stripeService.UpdateSubscriptionQuantity which
// requires a real Stripe connection. We test the validation logic only.

func TestBulkLicenseService_UpdateBatchQuantity_BatchNotFound(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	err := svc.UpdateBatchQuantity(uuid.New(), "some-user", 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "batch not found")
}

func TestBulkLicenseService_UpdateBatchQuantity_AccessDenied(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-010"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 5, 0)

	err := svc.UpdateBatchQuantity(batch.ID, "unauthorized-user", 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestBulkLicenseService_UpdateBatchQuantity_BelowAssigned(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-011"

	// 5 licenses, 3 assigned
	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 5, 3)

	// Try to reduce to 2, but 3 are assigned
	err := svc.UpdateBatchQuantity(batch.ID, purchaserID, 2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reduce quantity below assigned licenses")
}

// --- PermanentlyDeleteBatch tests ---

func TestBulkLicenseService_PermanentlyDeleteBatch_AccessDenied(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-012"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 0)

	err := svc.PermanentlyDeleteBatch(batch.ID, "unauthorized-user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestBulkLicenseService_PermanentlyDeleteBatch_NotFound(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)

	err := svc.PermanentlyDeleteBatch(uuid.New(), "some-user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "batch not found")
}

// --- Multiple assignments test ---

func TestBulkLicenseService_AssignLicense_MultipleAssignments(t *testing.T) {
	db := setupBulkLicenseTestDB(t)
	svc := services.NewBulkLicenseService(db)
	purchaserID := "purchaser-013"

	_, batch, _ := seedBulkLicenseTestData(t, db, purchaserID, 3, 0)

	// Assign all 3 licenses
	for i := 0; i < 3; i++ {
		targetUser := "user-" + uuid.New().String()[:8]
		license, err := svc.AssignLicense(batch.ID, purchaserID, targetUser)
		require.NoError(t, err)
		assert.Equal(t, targetUser, license.UserID)
	}

	// 4th assignment should fail
	_, err := svc.AssignLicense(batch.ID, purchaserID, "one-too-many")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no available licenses")

	// Verify batch quantity
	var updatedBatch models.SubscriptionBatch
	require.NoError(t, db.First(&updatedBatch, batch.ID).Error)
	assert.Equal(t, 3, updatedBatch.AssignedQuantity)
}
