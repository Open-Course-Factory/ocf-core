// tests/payment/planCatalogSeed_test.go
//
// Pins the seeded catalogue to the offer that was actually decided:
// Découverte (free) / Solo 12€ / Formateur 39€, plus the two hidden learner-seat
// products sold on graduated tiers.
//
// The seed had drifted from the decision by a year: it still created "Member Pro"
// and "Trainer Plan" at 12€ each, carried the superseded per-licence ladder
// (12/10/8/6) on the trainer plan, capped Solo sessions at 180 minutes — a
// documented launch blocker, since a training day is 7 hours — and knew nothing
// about learner seats, which existed only where someone had created them by hand.
package payment_tests

import (
	"testing"

	"soli/formations/src/initialization"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seededPlan(t *testing.T, db *gorm.DB, name string) *models.SubscriptionPlan {
	t.Helper()

	var plan models.SubscriptionPlan
	require.NoError(t, db.Where("name = ?", name).First(&plan).Error,
		"the seed must create the %q plan", name)
	return &plan
}

// TestSeed_PublicCatalogueMatchesTheDecidedOffer covers the three SKUs a visitor
// can actually buy.
func TestSeed_PublicCatalogueMatchesTheDecidedOffer(t *testing.T) {
	db := freshTestDB(t)

	initialization.SetupDefaultSubscriptionPlans(db)

	decouverte := seededPlan(t, db, "Découverte")
	assert.EqualValues(t, 0, decouverte.PriceAmount, "Découverte is free, permanently")
	assert.True(t, decouverte.IsCatalog)
	assert.False(t, decouverte.NetworkAccessEnabled, "no network on the free tier")
	assert.Equal(t, 60, decouverte.MaxSessionDurationMinutes)
	// One XS machine, ephemeral: the free tier must stay unattractive to abuse.
	assert.Equal(t, 500, decouverte.MaxCPU)
	assert.Equal(t, 256, decouverte.MaxMemoryMB)
	assert.False(t, decouverte.DataPersistenceEnabled)

	solo := seededPlan(t, db, "Solo")
	assert.EqualValues(t, 1200, solo.PriceAmount, "Solo is 12€/month")
	assert.True(t, solo.IsCatalog)
	// 8 hours, not 180 minutes: a real training day is 7 hours, and the old cap
	// was flagged as a launch blocker rather than a preference.
	assert.Equal(t, 480, solo.MaxSessionDurationMinutes)
	// The XL ceiling decided on 2026-07-30.
	assert.Equal(t, 6000, solo.MaxCPU)
	assert.Equal(t, 6144, solo.MaxMemoryMB)
	assert.True(t, solo.NetworkAccessEnabled)
	assert.True(t, solo.DataPersistenceEnabled)
	assert.False(t, solo.GroupManagementEnabled, "Solo runs no classrooms")

	formateur := seededPlan(t, db, "Formateur")
	// 19,90 € and not the 39 € of the April offer: that 39 € priced a pay-per-session
	// credit (3 days × 12 learners). July made Formateur a flat monthly base and moved
	// learners out into their own seat products, so the base price followed. Confirmed
	// against the configured Stripe sandbox on 2026-08-18.
	assert.EqualValues(t, 1990, formateur.PriceAmount, "Formateur is 19,90€/month")
	assert.True(t, formateur.IsCatalog)
	assert.Equal(t, 480, formateur.MaxSessionDurationMinutes)
	assert.True(t, formateur.GroupManagementEnabled, "Formateur is the classroom plan")
	assert.True(t, formateur.SessionSupervisionEnabled, "supervision is a trainer capability")
}

// TestSeed_FormateurNoLongerCarriesThePerLicenceLadder: learners are sold as
// their own seat products now. Leaving the old ladder on Formateur would price
// the same thing twice, in two shapes, from two rows.
func TestSeed_FormateurNoLongerCarriesThePerLicenceLadder(t *testing.T) {
	db := freshTestDB(t)

	initialization.SetupDefaultSubscriptionPlans(db)

	formateur := seededPlan(t, db, "Formateur")
	assert.False(t, formateur.UseTieredPricing,
		"the trainer's own subscription is a flat 39€ — seats carry the tiers")
	assert.Empty(t, formateur.PricingTiers)
	assert.False(t, formateur.BulkPurchasable,
		"Formateur is bought for oneself, not in bulk for learners")
}

// TestSeed_LearnerSeatProducts pins both ways a seat is sold, and the graduated
// ladders decided on 2026-07-30.
func TestSeed_LearnerSeatProducts(t *testing.T) {
	db := freshTestDB(t)

	initialization.SetupDefaultSubscriptionPlans(db)

	monthly := seededPlan(t, db, "Siège élève — mensuel")
	assert.False(t, monthly.IsCatalog, "seats are sold by a trainer, never off the pricing page")
	assert.True(t, monthly.BulkPurchasable)
	assert.Equal(t, models.SeatUnitSeatMonth, monthly.EffectiveSeatUnit())
	assert.True(t, monthly.UseTieredPricing)
	assert.False(t, monthly.GroupManagementEnabled,
		"a learner holding a seat must not inherit the right to buy seats")
	require.Len(t, monthly.PricingTiers, 3)
	assert.EqualValues(t, 900, monthly.PricingTiers[0].UnitAmount)
	assert.EqualValues(t, 700, monthly.PricingTiers[1].UnitAmount)
	assert.EqualValues(t, 550, monthly.PricingTiers[2].UnitAmount)

	pack := seededPlan(t, db, "Siège élève — pack jours")
	assert.False(t, pack.IsCatalog)
	assert.True(t, pack.BulkPurchasable)
	assert.Equal(t, models.SeatUnitLearnerDay, pack.EffectiveSeatUnit())
	assert.True(t, pack.UseTieredPricing)
	require.Len(t, pack.PricingTiers, 3)
	assert.EqualValues(t, 165, pack.PricingTiers[0].UnitAmount)
	assert.EqualValues(t, 125, pack.PricingTiers[1].UnitAmount)
	assert.EqualValues(t, 105, pack.PricingTiers[2].UnitAmount)
}

// TestSeed_TiersAreGraduatedAndContiguous guards the property the shape was
// chosen for: volume tiering made 11 seats cheaper than 10, so every ladder must
// stay monotonically decreasing with no gap or overlap between brackets.
func TestSeed_TiersAreGraduatedAndContiguous(t *testing.T) {
	db := freshTestDB(t)

	initialization.SetupDefaultSubscriptionPlans(db)

	for _, name := range []string{"Siège élève — mensuel", "Siège élève — pack jours"} {
		tiers := seededPlan(t, db, name).PricingTiers
		for i, tier := range tiers {
			if i == 0 {
				assert.Equal(t, 1, tier.MinQuantity, "%s: the first bracket starts at 1", name)
				continue
			}
			assert.Equal(t, tiers[i-1].MaxQuantity+1, tier.MinQuantity,
				"%s: bracket %d must start where the previous one ended", name, i)
			assert.Less(t, tier.UnitAmount, tiers[i-1].UnitAmount,
				"%s: bracket %d must be cheaper per unit than the one before", name, i)
		}
		assert.EqualValues(t, 0, tiers[len(tiers)-1].MaxQuantity,
			"%s: the last bracket is unbounded", name)
	}
}

// TestRenameLegacyPlans_KeepsTheRow is what makes this deployable: the plans in
// production are the same commercial products under their old names, carrying
// live subscriptions and Stripe ids. Renaming must UPDATE those rows, never
// create a second one beside them.
func TestRenameLegacyPlans_KeepsTheRow(t *testing.T) {
	db := freshTestDB(t)

	legacy := &models.SubscriptionPlan{
		Name:        "Member Pro",
		PriceAmount: 1200,
		Currency:    "eur",
		IsActive:    true,
	}
	require.NoError(t, db.Create(legacy).Error)

	initialization.RenameLegacyPlans(db)

	var reloaded models.SubscriptionPlan
	require.NoError(t, db.First(&reloaded, "id = ?", legacy.ID).Error)
	assert.Equal(t, "Solo", reloaded.Name, "the row is renamed in place")

	var count int64
	db.Model(&models.SubscriptionPlan{}).Where("name IN ?", []string{"Solo", "Member Pro"}).Count(&count)
	assert.EqualValues(t, 1, count, "renaming must not leave a duplicate behind")
}

// TestRenameLegacyPlans_FreePlanStaysFindable is the one that would break
// production silently: FindFreePlan looks the plan up BY NAME, so renaming the
// row without renaming the constant — or the constant without the row — leaves
// every new signup with no plan at all.
func TestRenameLegacyPlans_FreePlanStaysFindable(t *testing.T) {
	db := freshTestDB(t)

	require.NoError(t, db.Create(&models.SubscriptionPlan{
		Name:        "Trial",
		PriceAmount: 0,
		Currency:    "eur",
		IsActive:    true,
	}).Error)

	// The startup order: rename the row, then elect it. Election is deliberately
	// not part of the rename — a plan can be renamed without changing which plan
	// new signups receive.
	initialization.RenameLegacyPlans(db)
	initialization.MarkDefaultFreePlan(db)

	plan, err := services.FindFreePlan(db)
	require.NoError(t, err, "the free plan must still resolve after the rename")
	assert.Equal(t, services.FreePlanName, plan.Name)
	assert.Equal(t, "Découverte", plan.Name)
}

// TestRenameLegacyPlans_Idempotent: it runs on every startup, on every pod.
func TestRenameLegacyPlans_Idempotent(t *testing.T) {
	db := freshTestDB(t)

	require.NoError(t, db.Create(&models.SubscriptionPlan{
		Name: "Trainer Plan", PriceAmount: 1200, Currency: "eur", IsActive: true,
	}).Error)

	initialization.RenameLegacyPlans(db)
	initialization.RenameLegacyPlans(db)

	var count int64
	db.Model(&models.SubscriptionPlan{}).Where("name = ?", "Formateur").Count(&count)
	assert.EqualValues(t, 1, count)
}

// TestRenameLegacyPlans_LeavesAnAlreadyNamedCatalogueAlone: a database seeded
// after this change has no legacy names, and the migration must not touch the
// plans it finds there — in particular it must not rewrite prices.
func TestRenameLegacyPlans_LeavesAnAlreadyNamedCatalogueAlone(t *testing.T) {
	db := freshTestDB(t)

	initialization.SetupDefaultSubscriptionPlans(db)
	before := seededPlan(t, db, "Formateur")

	initialization.RenameLegacyPlans(db)

	after := seededPlan(t, db, "Formateur")
	assert.Equal(t, before.ID, after.ID)
	assert.Equal(t, before.PriceAmount, after.PriceAmount)
}
