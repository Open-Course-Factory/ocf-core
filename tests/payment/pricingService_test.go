package payment_tests

import (
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPricingTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.SubscriptionPlan{})
	require.NoError(t, err)

	return db
}

func createTestPlan(t *testing.T, db *gorm.DB, plan *models.SubscriptionPlan) *models.SubscriptionPlan {
	plan.BaseModel = entityManagementModels.BaseModel{ID: uuid.New()}
	err := db.Create(plan).Error
	require.NoError(t, err)
	return plan
}

// --- CalculatePricingPreview tests ---

func TestPricingService_CalculatePricingPreview_FlatPricing(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	plan := createTestPlan(t, db, &models.SubscriptionPlan{
		Name:             "Pro",
		PriceAmount:      1000, // 10.00 EUR
		Currency:         "eur",
		UseTieredPricing: false,
	})

	breakdown, err := svc.CalculatePricingPreview(plan.ID, 5)
	require.NoError(t, err)

	assert.Equal(t, "Pro", breakdown.PlanName)
	assert.Equal(t, 5, breakdown.TotalQuantity)
	assert.Equal(t, "eur", breakdown.Currency)
	assert.Equal(t, int64(5000), breakdown.TotalMonthlyCost) // 5 * 1000
	assert.Equal(t, 10.0, breakdown.AveragePerUnit)          // 1000 / 100.0
	assert.Equal(t, int64(0), breakdown.Savings)
	assert.Len(t, breakdown.TierBreakdown, 1)
	assert.Equal(t, 5, breakdown.TierBreakdown[0].Quantity)
	assert.Equal(t, int64(1000), breakdown.TierBreakdown[0].UnitPrice)
	assert.Equal(t, int64(5000), breakdown.TierBreakdown[0].Subtotal)
}

func TestPricingService_CalculatePricingPreview_TieredPricing(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	// Tiers: 1-5 @ 1000, 6-15 @ 800, 16+ @ 600
	plan := createTestPlan(t, db, &models.SubscriptionPlan{
		Name:             "Business",
		PriceAmount:      1000, // base/flat price
		Currency:         "eur",
		UseTieredPricing: true,
		PricingTiers: []models.PricingTier{
			{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 1000},
			{MinQuantity: 6, MaxQuantity: 15, UnitAmount: 800},
			{MinQuantity: 16, MaxQuantity: 0, UnitAmount: 600}, // unlimited
		},
	})

	// 20 licenses: 5*1000 + 10*800 + 5*600 = 5000 + 8000 + 3000 = 16000
	breakdown, err := svc.CalculatePricingPreview(plan.ID, 20)
	require.NoError(t, err)

	assert.Equal(t, "Business", breakdown.PlanName)
	assert.Equal(t, 20, breakdown.TotalQuantity)
	assert.Equal(t, int64(16000), breakdown.TotalMonthlyCost)
	assert.Len(t, breakdown.TierBreakdown, 3)

	// Tier 1: 5 licenses at 1000
	assert.Equal(t, 5, breakdown.TierBreakdown[0].Quantity)
	assert.Equal(t, int64(1000), breakdown.TierBreakdown[0].UnitPrice)
	assert.Equal(t, int64(5000), breakdown.TierBreakdown[0].Subtotal)

	// Tier 2: 10 licenses at 800
	assert.Equal(t, 10, breakdown.TierBreakdown[1].Quantity)
	assert.Equal(t, int64(800), breakdown.TierBreakdown[1].UnitPrice)
	assert.Equal(t, int64(8000), breakdown.TierBreakdown[1].Subtotal)

	// Tier 3: 5 licenses at 600 (unlimited tier)
	assert.Equal(t, 5, breakdown.TierBreakdown[2].Quantity)
	assert.Equal(t, int64(600), breakdown.TierBreakdown[2].UnitPrice)
	assert.Equal(t, int64(3000), breakdown.TierBreakdown[2].Subtotal)
	assert.Equal(t, "16+", breakdown.TierBreakdown[2].Range)

	// Savings = individual cost - tiered cost = 20*1000 - 16000 = 4000
	assert.Equal(t, int64(4000), breakdown.Savings)

	// Average per unit = 16000 / 20 / 100 = 8.0
	assert.InDelta(t, 8.0, breakdown.AveragePerUnit, 0.01)
}

func TestPricingService_CalculatePricingPreview_QuantityExceedingAllTiers(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	plan := createTestPlan(t, db, &models.SubscriptionPlan{
		Name:             "Tiered",
		PriceAmount:      1000,
		Currency:         "eur",
		UseTieredPricing: true,
		PricingTiers: []models.PricingTier{
			{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 1000},
			{MinQuantity: 6, MaxQuantity: 0, UnitAmount: 700}, // unlimited
		},
	})

	// 100 licenses: 5*1000 + 95*700 = 5000 + 66500 = 71500
	breakdown, err := svc.CalculatePricingPreview(plan.ID, 100)
	require.NoError(t, err)

	assert.Equal(t, int64(71500), breakdown.TotalMonthlyCost)
	assert.Len(t, breakdown.TierBreakdown, 2)
	assert.Equal(t, 5, breakdown.TierBreakdown[0].Quantity)
	assert.Equal(t, 95, breakdown.TierBreakdown[1].Quantity)
}

func TestPricingService_CalculatePricingPreview_SingleTierOnly(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	plan := createTestPlan(t, db, &models.SubscriptionPlan{
		Name:             "Flat Tiered",
		PriceAmount:      500,
		Currency:         "eur",
		UseTieredPricing: true,
		PricingTiers: []models.PricingTier{
			{MinQuantity: 1, MaxQuantity: 0, UnitAmount: 500}, // single unlimited tier
		},
	})

	breakdown, err := svc.CalculatePricingPreview(plan.ID, 10)
	require.NoError(t, err)

	assert.Equal(t, int64(5000), breakdown.TotalMonthlyCost)
	assert.Len(t, breakdown.TierBreakdown, 1)
	assert.Equal(t, 10, breakdown.TierBreakdown[0].Quantity)
	assert.Equal(t, int64(500), breakdown.TierBreakdown[0].UnitPrice)
	// Savings = 10*500 - 5000 = 0 (same as flat price)
	assert.Equal(t, int64(0), breakdown.Savings)
}

func TestPricingService_CalculatePricingPreview_BoundaryAtTierTransition(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	plan := createTestPlan(t, db, &models.SubscriptionPlan{
		Name:             "Boundary",
		PriceAmount:      1000,
		Currency:         "eur",
		UseTieredPricing: true,
		PricingTiers: []models.PricingTier{
			{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 1000},
			{MinQuantity: 6, MaxQuantity: 10, UnitAmount: 800},
			{MinQuantity: 11, MaxQuantity: 0, UnitAmount: 600},
		},
	})

	// Exactly 5 licenses: all in first tier
	breakdown, err := svc.CalculatePricingPreview(plan.ID, 5)
	require.NoError(t, err)
	assert.Equal(t, int64(5000), breakdown.TotalMonthlyCost)
	assert.Len(t, breakdown.TierBreakdown, 1)

	// Exactly 6 licenses: 5 in tier 1, 1 in tier 2
	breakdown, err = svc.CalculatePricingPreview(plan.ID, 6)
	require.NoError(t, err)
	assert.Equal(t, int64(5800), breakdown.TotalMonthlyCost) // 5*1000 + 1*800
	assert.Len(t, breakdown.TierBreakdown, 2)

	// Exactly 10 licenses: 5 in tier 1, 5 in tier 2
	breakdown, err = svc.CalculatePricingPreview(plan.ID, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(9000), breakdown.TotalMonthlyCost) // 5*1000 + 5*800
	assert.Len(t, breakdown.TierBreakdown, 2)

	// Exactly 11 licenses: 5 in tier 1, 5 in tier 2, 1 in tier 3
	breakdown, err = svc.CalculatePricingPreview(plan.ID, 11)
	require.NoError(t, err)
	assert.Equal(t, int64(9600), breakdown.TotalMonthlyCost) // 5*1000 + 5*800 + 1*600
	assert.Len(t, breakdown.TierBreakdown, 3)
}

func TestPricingService_CalculatePricingPreview_PlanNotFound(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	_, err := svc.CalculatePricingPreview(uuid.New(), 5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plan not found")
}

func TestPricingService_CalculatePricingPreview_EmptyTiersUsesFlatPricing(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	// UseTieredPricing is true but no tiers defined -> falls back to flat
	plan := createTestPlan(t, db, &models.SubscriptionPlan{
		Name:             "Empty Tiers",
		PriceAmount:      1500,
		Currency:         "eur",
		UseTieredPricing: true,
		PricingTiers:     []models.PricingTier{},
	})

	breakdown, err := svc.CalculatePricingPreview(plan.ID, 3)
	require.NoError(t, err)

	assert.Equal(t, int64(4500), breakdown.TotalMonthlyCost) // 3 * 1500
	assert.Equal(t, int64(0), breakdown.Savings)
	assert.Len(t, breakdown.TierBreakdown, 1)
}

// --- GetTotalCost tests ---

func TestPricingService_GetTotalCost_FlatPricing(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	plan := &models.SubscriptionPlan{
		PriceAmount:      1000,
		UseTieredPricing: false,
	}

	cost := svc.GetTotalCost(plan, 10)
	assert.Equal(t, int64(10000), cost)
}

func TestPricingService_GetTotalCost_TieredPricing(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	plan := &models.SubscriptionPlan{
		PriceAmount:      1000,
		UseTieredPricing: true,
		PricingTiers: []models.PricingTier{
			{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 1000},
			{MinQuantity: 6, MaxQuantity: 15, UnitAmount: 800},
			{MinQuantity: 16, MaxQuantity: 0, UnitAmount: 600},
		},
	}

	// 20 licenses: 5*1000 + 10*800 + 5*600 = 16000
	cost := svc.GetTotalCost(plan, 20)
	assert.Equal(t, int64(16000), cost)
}

func TestPricingService_GetTotalCost_ConsistencyWithPreview(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	// Flat pricing consistency
	flatPlan := createTestPlan(t, db, &models.SubscriptionPlan{
		Name:             "Flat",
		PriceAmount:      1200,
		Currency:         "eur",
		UseTieredPricing: false,
	})

	breakdown, err := svc.CalculatePricingPreview(flatPlan.ID, 7)
	require.NoError(t, err)
	cost := svc.GetTotalCost(flatPlan, 7)
	assert.Equal(t, breakdown.TotalMonthlyCost, cost)

	// Tiered pricing consistency
	tieredPlan := createTestPlan(t, db, &models.SubscriptionPlan{
		Name:             "Tiered",
		PriceAmount:      1000,
		Currency:         "eur",
		UseTieredPricing: true,
		PricingTiers: []models.PricingTier{
			{MinQuantity: 1, MaxQuantity: 10, UnitAmount: 1000},
			{MinQuantity: 11, MaxQuantity: 0, UnitAmount: 750},
		},
	})

	breakdown, err = svc.CalculatePricingPreview(tieredPlan.ID, 25)
	require.NoError(t, err)
	cost = svc.GetTotalCost(tieredPlan, 25)
	assert.Equal(t, breakdown.TotalMonthlyCost, cost)
}

func TestPricingService_GetTotalCost_ZeroQuantity(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	plan := &models.SubscriptionPlan{
		PriceAmount:      1000,
		UseTieredPricing: false,
	}

	cost := svc.GetTotalCost(plan, 0)
	assert.Equal(t, int64(0), cost)
}

func TestPricingService_GetTotalCost_ZeroQuantityTiered(t *testing.T) {
	db := setupPricingTestDB(t)
	svc := services.NewPricingService(db)

	plan := &models.SubscriptionPlan{
		PriceAmount:      1000,
		UseTieredPricing: true,
		PricingTiers: []models.PricingTier{
			{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 1000},
			{MinQuantity: 6, MaxQuantity: 0, UnitAmount: 800},
		},
	}

	cost := svc.GetTotalCost(plan, 0)
	assert.Equal(t, int64(0), cost)
}
