// tests/payment/bulkPurchaseGateSplit_test.go
//
// #441: the bulk-purchase gate conflated two different questions into one check
// on the plan being bought — `IsCatalog && GroupManagementEnabled`.
//
// Both were wrong for a learner seat:
//
//   - IsCatalog means "listed on the public pricing page", so a seat plan could
//     not be both sellable in bulk and hidden from prospects.
//   - GroupManagementEnabled on the PURCHASED plan meant a trainer could only buy
//     licences of a plan that itself grants group management — so students would
//     inherit group management, and could themselves buy seats.
//
// The result was that a cheap learner-seat plan, priced below the individual
// plan, could not exist at all.
//
// The two questions are now asked separately: BulkPurchasable on the plan being
// sold, GroupManagementEnabled on the purchaser's own effective plan.
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedTrainerWithGroupManagement gives userID an entitling subscription on a plan
// that grants group management — i.e. a legitimate seat purchaser.
func seedTrainerWithGroupManagement(t *testing.T, db *gorm.DB, userID string) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Formateur (purchaser)",
		Currency:               "eur",
		PriceAmount:            1990,
		BillingInterval:        "month",
		IsActive:               true,
		Priority:               20,
		GroupManagementEnabled: true,
	}
	require.NoError(t, db.Create(plan).Error)

	now := time.Now()
	require.NoError(t, db.Create(&models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		SubscriptionType:   "personal",
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
	}).Error)
	return plan
}

// seedSeatPlan creates the learner-seat plan: hidden from the public catalogue,
// cheap, granting no group management, but explicitly sellable in bulk.
func seedSeatPlan(t *testing.T, db *gorm.DB) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Siège élève",
		Currency:        "eur",
		PriceAmount:     600,
		BillingInterval: "month",
		IsActive:        true,
		Priority:        5,
		BulkPurchasable: true,
		// GroupManagementEnabled deliberately false — students must not get it.
	}
	require.NoError(t, db.Create(plan).Error)
	// IsCatalog carries gorm:"default:true", and GORM omits a zero-value bool on
	// Create, so the DB default wins. Force it false explicitly — the same
	// footgun the other catalog tests work around.
	require.NoError(t, db.Model(plan).Update("is_catalog", false).Error)
	return plan
}

// TestBulkPurchase_HiddenSeatPlanIsPurchasableByATrainer is the case the old gate
// made impossible: a hidden, cheap seat plan that grants no group management,
// bought by a trainer whose own plan does.
func TestBulkPurchase_HiddenSeatPlanIsPurchasableByATrainer(t *testing.T) {
	db := freshTestDB(t)
	installFakeCasdoor(t, "trainer-seats@example.com", "Trainer Buying Seats")
	svc := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{})

	const buyer = "buyer-trainer-seats"
	seedTrainerWithGroupManagement(t, db, buyer)
	seatPlan := seedSeatPlan(t, db)

	batch, licenses, err := svc.PurchaseBulkLicenses(buyer, dto.BulkPurchaseInput{
		SubscriptionPlanID: seatPlan.ID,
		Quantity:           12,
	})

	require.NoError(t, err,
		"a hidden seat plan marked BulkPurchasable must be sellable to a trainer whose own "+
			"plan grants group management")
	require.NotNil(t, batch)
	require.NotNil(t, licenses)
	assert.Len(t, *licenses, 12, "one licence per seat purchased")
}

// TestBulkPurchase_SeatPlanNeedNotGrantGroupManagement pins the entitlement
// consequence: the licences handed to students must not carry group management,
// which the old gate forced onto every bulk-purchasable plan.
func TestBulkPurchase_SeatPlanNeedNotGrantGroupManagement(t *testing.T) {
	db := freshTestDB(t)
	installFakeCasdoor(t, "trainer-nogm@example.com", "Trainer")
	svc := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{})

	const buyer = "buyer-seat-nogm"
	seedTrainerWithGroupManagement(t, db, buyer)
	seatPlan := seedSeatPlan(t, db)
	require.False(t, seatPlan.GroupManagementEnabled,
		"precondition: the seat plan grants no group management")

	_, _, err := svc.PurchaseBulkLicenses(buyer, dto.BulkPurchaseInput{
		SubscriptionPlanID: seatPlan.ID,
		Quantity:           2,
	})
	require.NoError(t, err)

	var sold models.SubscriptionPlan
	require.NoError(t, db.Where("id = ?", seatPlan.ID).First(&sold).Error)
	assert.False(t, sold.GroupManagementEnabled,
		"selling seats must not require granting students group management")
}

// TestBulkPurchase_PurchaserWithoutGroupManagement_Rejected moves the old gate to
// where it belongs: the buyer. A learner holding a seat must not be able to turn
// around and buy seats of their own.
func TestBulkPurchase_PurchaserWithoutGroupManagement_Rejected(t *testing.T) {
	db := freshTestDB(t)
	installFakeCasdoor(t, "learner-buyer@example.com", "Learner")
	svc := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{})

	const buyer = "buyer-learner"
	seatPlan := seedSeatPlan(t, db)

	// The buyer holds a seat: entitling, but granting no group management.
	now := time.Now()
	require.NoError(t, db.Create(&models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             buyer,
		SubscriptionPlanID: seatPlan.ID,
		Status:             "active",
		SubscriptionType:   "personal",
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
	}).Error)

	batch, licenses, err := svc.PurchaseBulkLicenses(buyer, dto.BulkPurchaseInput{
		SubscriptionPlanID: seatPlan.ID,
		Quantity:           5,
	})

	require.Error(t, err,
		"PURCHASER: group management is a property of the buyer's plan, not the plan being "+
			"bought — a seat holder must not be able to buy seats")
	assert.Nil(t, batch)
	assert.Nil(t, licenses)
}

// TestBulkPurchase_PlanNotMarkedBulkPurchasable_Rejected replaces the old
// IsCatalog gate. Visibility and sellability are now separate: an ordinary
// individual plan is not a seat product just because it is public.
func TestBulkPurchase_PlanNotMarkedBulkPurchasable_Rejected(t *testing.T) {
	db := freshTestDB(t)
	installFakeCasdoor(t, "trainer-wrongplan@example.com", "Trainer")
	svc := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{})

	const buyer = "buyer-wrongplan"
	seedTrainerWithGroupManagement(t, db, buyer)

	soloPlan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Solo (not a seat product)",
		Currency:        "eur",
		PriceAmount:     1200,
		BillingInterval: "month",
		IsActive:        true,
		Priority:        10,
		// BulkPurchasable deliberately false.
	}
	require.NoError(t, db.Create(soloPlan).Error)

	batch, licenses, err := svc.PurchaseBulkLicenses(buyer, dto.BulkPurchaseInput{
		SubscriptionPlanID: soloPlan.ID,
		Quantity:           4,
	})

	require.Error(t, err,
		"a plan not marked BulkPurchasable must not be sellable in bulk, however visible it is")
	assert.Nil(t, batch)
	assert.Nil(t, licenses)
	// Not assertNoBulkRowsPersisted: that counts every UserSubscription row, and
	// the purchaser legitimately holds one. Assert on batches, which is what a
	// rejected bulk purchase must not create.
	var batchCount int64
	require.NoError(t, db.Model(&models.SubscriptionBatch{}).Count(&batchCount).Error)
	assert.Equal(t, int64(0), batchCount, "a rejected bulk purchase must persist no batch")
}

// TestBulkPurchase_InactivePlan_StillRejected keeps the surviving half of the
// original gate honest.
func TestBulkPurchase_InactivePlan_StillRejected(t *testing.T) {
	db := freshTestDB(t)
	installFakeCasdoor(t, "trainer-inactive@example.com", "Trainer")
	svc := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{})

	const buyer = "buyer-inactive"
	seedTrainerWithGroupManagement(t, db, buyer)

	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Retired seat plan",
		Currency:        "eur",
		PriceAmount:     600,
		BillingInterval: "month",
		BulkPurchasable: true,
	}
	require.NoError(t, db.Create(plan).Error)
	require.NoError(t, db.Model(plan).Update("is_active", false).Error)

	_, _, err := svc.PurchaseBulkLicenses(buyer, dto.BulkPurchaseInput{
		SubscriptionPlanID: plan.ID,
		Quantity:           1,
	})
	require.Error(t, err, "an inactive plan must never be bulk-purchasable")
}
