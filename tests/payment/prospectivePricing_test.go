// tests/payment/prospectivePricing_test.go
//
// #445: pricing a tier set that has not been saved yet.
//
// An admin editing brackets could not see what they produce until after saving,
// which is backwards for the operation most likely to be got wrong: graduated
// brackets are hard to reason about, and a first bracket wider than a typical
// order silently gives most customers no discount at all.
//
// These tests also pin the extraction of the graduated engine. It was written
// twice in pricingService.go — CalculatePricingPreview and GetTotalCost each
// walked the tiers — so "the two agree" is a property worth holding onto.
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ladders agreed for the seat offer (see ocf-core #442).
func monthlySeatTiers() []models.PricingTier {
	return []models.PricingTier{
		{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 900},
		{MinQuantity: 6, MaxQuantity: 15, UnitAmount: 700},
		{MinQuantity: 16, MaxQuantity: 0, UnitAmount: 550},
	}
}

func dayPackTiers() []models.PricingTier {
	return []models.PricingTier{
		{MinQuantity: 1, MaxQuantity: 30, UnitAmount: 165},
		{MinQuantity: 31, MaxQuantity: 60, UnitAmount: 125},
		{MinQuantity: 61, MaxQuantity: 0, UnitAmount: 105},
	}
}

// TestGraduatedCost_MatchesTheAgreedLadders pins the arithmetic against the
// numbers the offer was signed off on, so a change to the engine that quietly
// alters what customers pay fails here rather than in production.
func TestGraduatedCost_MatchesTheAgreedLadders(t *testing.T) {
	monthly := monthlySeatTiers()
	for _, tc := range []struct {
		seats int
		total int64
	}{
		{1, 900},    // inside bracket 1
		{5, 4500},   // exactly fills bracket 1
		{10, 8000},  // 5x900 + 5x700
		{15, 11500}, // 5x900 + 10x700
		{20, 14250}, // 5x900 + 10x700 + 5x550
	} {
		got, _ := services.GraduatedCost(monthly, tc.seats)
		assert.Equal(t, tc.total, got, "monthly ladder at %d seats", tc.seats)
	}

	pack := dayPackTiers()
	for _, tc := range []struct {
		learnerDays int
		total       int64
	}{
		{15, 2475},  // 5 seats x 3 days
		{30, 4950},  // 10 seats x 3 days — exactly fills bracket 1
		{45, 6825},  // 15 seats x 3 days
		{75, 10275}, // 15 seats x 5 days
	} {
		got, _ := services.GraduatedCost(pack, tc.learnerDays)
		assert.Equal(t, tc.total, got, "day pack at %d learner-days", tc.learnerDays)
	}
}

// TestGraduatedCost_EdgeQuantities covers the inputs that silently destroy state
// elsewhere in this codebase: zero and empty.
func TestGraduatedCost_EdgeQuantities(t *testing.T) {
	tiers := monthlySeatTiers()

	total, breakdown := services.GraduatedCost(tiers, 0)
	assert.Equal(t, int64(0), total, "zero units cost nothing")
	assert.Empty(t, breakdown, "zero units consume no bracket")

	total, breakdown = services.GraduatedCost(nil, 10)
	assert.Equal(t, int64(0), total, "no tiers means no tiered cost — callers fall back to flat")
	assert.Empty(t, breakdown)

	total, _ = services.GraduatedCost(tiers, -3)
	assert.Equal(t, int64(0), total, "a negative quantity must not produce a negative price")
}

// TestGraduatedCost_UnlimitedLastBracket pins that MaxQuantity=0 absorbs the rest
// rather than being read as an empty bracket.
func TestGraduatedCost_UnlimitedLastBracket(t *testing.T) {
	total, breakdown := services.GraduatedCost(monthlySeatTiers(), 100)
	// 5x900 + 10x700 + 85x550
	assert.Equal(t, int64(4500+7000+46750), total)
	require.Len(t, breakdown, 3)
	assert.Equal(t, 85, breakdown[2].Quantity, "the open bracket takes every remaining unit")
}

// TestPreviewProspectiveTiers_ReturnsAPointPerQuantity is the admin-editor
// contract: hand it brackets that exist nowhere in the database and get back the
// table the admin needs to judge them.
func TestPreviewProspectiveTiers_ReturnsAPointPerQuantity(t *testing.T) {
	svc := services.NewPricingService(freshTestDB(t))

	out, err := svc.PreviewProspectiveTiers(dto.ProspectivePricingInput{
		Tiers: []dto.PricingTier{
			{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 900},
			{MinQuantity: 6, MaxQuantity: 15, UnitAmount: 700},
			{MinQuantity: 16, MaxQuantity: 0, UnitAmount: 550},
		},
		Currency:   "eur",
		Quantities: []int{5, 10, 15},
	})
	require.NoError(t, err)
	require.Len(t, out.Points, 3)

	assert.Equal(t, "eur", out.Currency)
	assert.Equal(t, int64(4500), out.Points[0].Total)
	assert.Equal(t, int64(8000), out.Points[1].Total)
	assert.Equal(t, int64(11500), out.Points[2].Total)

	assert.InDelta(t, 9.00, out.Points[0].PerUnit, 0.005, "9.00 per seat at 5 seats")
	assert.InDelta(t, 8.00, out.Points[1].PerUnit, 0.005, "8.00 per seat at 10 seats")
	assert.InDelta(t, 7.67, out.Points[2].PerUnit, 0.005, "7.67 per seat at 15 seats")
}

// TestPreviewProspectiveTiers_FlatWhenNoTiers keeps the untiered case honest —
// an admin clearing the brackets must see the flat price, not zero.
func TestPreviewProspectiveTiers_FlatWhenNoTiers(t *testing.T) {
	svc := services.NewPricingService(freshTestDB(t))

	out, err := svc.PreviewProspectiveTiers(dto.ProspectivePricingInput{
		FlatAmount: 1200,
		Currency:   "eur",
		Quantities: []int{1, 10},
	})
	require.NoError(t, err)
	require.Len(t, out.Points, 2)
	assert.Equal(t, int64(1200), out.Points[0].Total)
	assert.Equal(t, int64(12000), out.Points[1].Total)
}

// TestPreviewProspectiveTiers_RejectsEmptyQuantities — an empty request is a
// caller bug, and returning an empty table silently would read as "these
// brackets cost nothing".
func TestPreviewProspectiveTiers_RejectsEmptyQuantities(t *testing.T) {
	svc := services.NewPricingService(freshTestDB(t))

	_, err := svc.PreviewProspectiveTiers(dto.ProspectivePricingInput{
		Tiers:      []dto.PricingTier{{MinQuantity: 1, MaxQuantity: 0, UnitAmount: 900}},
		Quantities: nil,
	})
	assert.Error(t, err, "no quantities to price must be an explicit error")
}

// TestSavedPlanPreviewAgreesWithProspective is the anti-drift guard for the
// extraction: the saved-plan path and the prospective path must be the same
// engine, or the admin's preview and the customer's invoice diverge — exactly
// the failure #442 exists to fix.
func TestSavedPlanPreviewAgreesWithProspective(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewPricingService(db)

	plan := &models.SubscriptionPlan{
		Name:             "Seat ladder under test",
		Currency:         "eur",
		BillingInterval:  "month",
		IsActive:         true,
		PriceAmount:      900,
		UseTieredPricing: true,
		PricingTiers:     monthlySeatTiers(),
	}
	require.NoError(t, db.Create(plan).Error)

	for _, qty := range []int{1, 5, 10, 15, 42} {
		saved, err := svc.CalculatePricingPreview(plan.ID, qty)
		require.NoError(t, err)

		prospective, err := svc.PreviewProspectiveTiers(dto.ProspectivePricingInput{
			Tiers: []dto.PricingTier{
				{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 900},
				{MinQuantity: 6, MaxQuantity: 15, UnitAmount: 700},
				{MinQuantity: 16, MaxQuantity: 0, UnitAmount: 550},
			},
			FlatAmount: 900,
			Currency:   "eur",
			Quantities: []int{qty},
		})
		require.NoError(t, err)

		assert.Equal(t, saved.TotalMonthlyCost, prospective.Points[0].Total,
			"saved-plan and prospective pricing must agree at %d units", qty)
		assert.Equal(t, saved.TotalMonthlyCost, svc.GetTotalCost(plan, qty),
			"GetTotalCost must agree with CalculatePricingPreview at %d units", qty)
	}
}
