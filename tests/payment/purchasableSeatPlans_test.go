// tests/payment/purchasableSeatPlans_test.go
//
// ocf-front#296 needs the trainer to SEE the seat products before buying them,
// and they are deliberately hidden: a learner seat is is_catalog=false so it
// never reaches the public pricing page. VisibilityScope therefore hides it from
// every non-admin — including the trainer it exists for.
//
// The listing must return exactly the set a purchase would accept, so it reuses
// the same two predicates as the purchase path rather than restating them. A
// list that disagrees with the gate is worse than no list: it offers a product
// and then refuses the sale.
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
	"gorm.io/gorm"
)

func seedSellableSeat(t *testing.T, db *gorm.DB, name string, active bool) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             name,
		Currency:         "eur",
		BillingInterval:  "month",
		PriceAmount:      900,
		BulkPurchasable:  true,
		UseTieredPricing: true,
		PricingTiers: []models.PricingTier{
			{MinQuantity: 1, MaxQuantity: 5, UnitAmount: 900},
			{MinQuantity: 6, MaxQuantity: 0, UnitAmount: 700},
		},
	}
	require.NoError(t, db.Create(plan).Error)
	// Hidden from the catalogue, and inactive when asked: both are zero-value
	// bools on a default:true column, so Create cannot set them (#447).
	updates := map[string]interface{}{"is_catalog": false}
	if !active {
		updates["is_active"] = false
	}
	require.NoError(t, db.Model(plan).Updates(updates).Error)
	return plan
}

// TestPurchasableSeats_TrainerSeesHiddenSeatProducts is the point: the products
// are hidden from the catalogue and must still be offered to their buyer.
func TestPurchasableSeats_TrainerSeesHiddenSeatProducts(t *testing.T) {
	db := freshTestDB(t)
	const buyer = "buyer-sees-seats"
	seedTrainerWithGroupManagement(t, db, buyer)
	seedSellableSeat(t, db, "Siège élève — mensuel", true)
	seedSellableSeat(t, db, "Siège élève — pack jours", true)

	out, err := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{}).
		ListPurchasableSeatPlans(buyer)
	require.NoError(t, err)

	assert.True(t, out.CanPurchase, "a trainer whose plan grants group management may buy seats")
	names := make([]string, 0, len(out.Plans))
	for _, p := range out.Plans {
		names = append(names, p.Name)
	}
	assert.ElementsMatch(t, []string{"Siège élève — mensuel", "Siège élève — pack jours"}, names)
}

// TestPurchasableSeats_CarriesTheLadder — the UI prices the order from this, so
// an entry without its brackets is useless.
func TestPurchasableSeats_CarriesTheLadder(t *testing.T) {
	db := freshTestDB(t)
	const buyer = "buyer-ladder"
	seedTrainerWithGroupManagement(t, db, buyer)
	seedSellableSeat(t, db, "Siège élève — mensuel", true)

	out, err := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{}).
		ListPurchasableSeatPlans(buyer)
	require.NoError(t, err)
	require.Len(t, out.Plans, 1)

	seat := out.Plans[0]
	assert.True(t, seat.UseTieredPricing)
	require.Len(t, seat.PricingTiers, 2, "the ladder must travel with the plan")
	assert.Equal(t, int64(900), seat.PricingTiers[0].UnitAmount)
	assert.Equal(t, int64(700), seat.PricingTiers[1].UnitAmount)
}

// TestPurchasableSeats_IneligibleBuyerGetsNothing keeps hidden plans hidden. A
// learner holding a seat must not be shown the seat catalogue, and must not be
// told they can buy.
func TestPurchasableSeats_IneligibleBuyerGetsNothing(t *testing.T) {
	db := freshTestDB(t)
	seat := seedSellableSeat(t, db, "Siège élève — mensuel", true)

	const learner = "learner-not-a-buyer"
	now := time.Now()
	require.NoError(t, db.Create(&models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             learner,
		SubscriptionPlanID: seat.ID,
		Status:             "active",
		SubscriptionType:   "personal",
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
	}).Error)

	out, err := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{}).
		ListPurchasableSeatPlans(learner)
	require.NoError(t, err)

	assert.False(t, out.CanPurchase, "a seat holder is not a seat buyer")
	assert.Empty(t, out.Plans,
		"hidden plans must stay hidden from callers who could not buy them anyway")
	assert.NotEmpty(t, out.Reason, "the UI needs to explain the refusal, not just show nothing")
}

// TestPurchasableSeats_ResolvesTheSeatUnit pins the field the purchase screen
// needs to turn an order into a quantity. Nothing else distinguishes the two seat
// products — both are billing_interval=month — so an unresolved unit leaves the
// screen unable to price "12 learners for 3 days" at all.
func TestPurchasableSeats_ResolvesTheSeatUnit(t *testing.T) {
	db := freshTestDB(t)
	const buyer = "buyer-units"
	seedTrainerWithGroupManagement(t, db, buyer)

	monthly := seedSellableSeat(t, db, "Siège élève — mensuel", true)
	pack := seedSellableSeat(t, db, "Siège élève — pack jours", true)
	require.NoError(t, db.Model(pack).Update("seat_unit", models.SeatUnitLearnerDay).Error)
	_ = monthly // left with an UNSET unit on purpose

	out, err := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{}).
		ListPurchasableSeatPlans(buyer)
	require.NoError(t, err)

	units := map[string]string{}
	for _, p := range out.Plans {
		units[p.Name] = p.SeatUnit
	}
	assert.Equal(t, models.SeatUnitLearnerDay, units["Siège élève — pack jours"])
	assert.Equal(t, models.SeatUnitSeatMonth, units["Siège élève — mensuel"],
		"an unset unit must resolve to seat_month — every plan predating the column is per-seat, "+
			"and the screen must never receive an empty unit to interpret")
}

// TestPurchasableSeats_OffersOnlyWhatAPurchaseWouldAccept is the anti-drift
// guard. Anything listed here must pass the purchase gate; a plan that is not a
// seat product, or is retired, must never be offered.
func TestPurchasableSeats_OffersOnlyWhatAPurchaseWouldAccept(t *testing.T) {
	db := freshTestDB(t)
	const buyer = "buyer-strict"
	seedTrainerWithGroupManagement(t, db, buyer)

	seedSellableSeat(t, db, "Siège élève — mensuel", true)
	seedSellableSeat(t, db, "Retired seat", false) // inactive

	// An ordinary individual plan: public, but not a seat product.
	solo := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Solo",
		Currency:        "eur",
		BillingInterval: "month",
		PriceAmount:     1200,
	}
	require.NoError(t, db.Create(solo).Error)

	out, err := services.NewBulkLicenseServiceWithDeps(db, &bulkGatesStripeStub{}).
		ListPurchasableSeatPlans(buyer)
	require.NoError(t, err)

	names := make([]string, 0, len(out.Plans))
	for _, p := range out.Plans {
		names = append(names, p.Name)
	}
	assert.Equal(t, []string{"Siège élève — mensuel"}, names,
		"only active, bulk-purchasable plans may be offered — listing anything else offers a "+
			"product the purchase gate would then refuse")
}
