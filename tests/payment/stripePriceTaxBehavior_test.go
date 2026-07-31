// tests/payment/stripePriceTaxBehavior_test.go
//
// #387: prices reached Stripe with tax_behavior unset, so whether OCF sells
// HT or TTC was decided by a Stripe Dashboard toggle rather than by the code.
//
// The product decision was already made and shipped — the public pricing page
// states "Prix hors taxes" / "Prices exclude VAT" (ocf-front PublicOffers.vue).
// Stripe never learned it. Left unspecified, the account default
// `inferred_by_currency` resolves EUR to *inclusive*, so a 19.90 EUR plan bills
// 16.58 HT + 3.32 VAT: the announced price silently becomes the gross, cutting
// every modelled figure by 16.7%.
//
// It stayed invisible because Stripe Tax was itself misconfigured and computed
// no VAT at all. Fixing the tax registration is what forced Stripe to resolve
// the ambiguity, and it resolved it the wrong way.
//
// tax_behavior can only be moved off "unspecified" once — after that a price is
// immutable and has to be replaced — which is why this belongs in code next to
// the amount, not in a dashboard.
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v85"
)

// TestApplyPricing_FlatPriceIsTaxExclusive — the announced amount is the HT
// amount; VAT is added on top.
func TestApplyPricing_FlatPriceIsTaxExclusive(t *testing.T) {
	params := &stripe.PriceParams{}

	services.ApplyPricing(params, &models.SubscriptionPlan{PriceAmount: 1990})

	require.NotNil(t, params.TaxBehavior, "a price with no tax behaviour lets the Stripe "+
		"account default decide whether we sell HT or TTC")
	assert.Equal(t, "exclusive", *params.TaxBehavior)
	assert.Equal(t, int64(1990), *params.UnitAmount)
}

// TestApplyPricing_TieredPriceIsTaxExclusive — the seat ladders bill the same
// way. This is the path that carries the per-student pricing, so an inclusive
// ladder would quietly shave 16.7% off every bracket.
func TestApplyPricing_TieredPriceIsTaxExclusive(t *testing.T) {
	params := &stripe.PriceParams{}
	plan := &models.SubscriptionPlan{
		UseTieredPricing: true,
		PricingTiers: []models.PricingTier{
			{MaxQuantity: 10, UnitAmount: 1200},
			{MaxQuantity: 0, UnitAmount: 900},
		},
	}

	services.ApplyPricing(params, plan)

	require.NotNil(t, params.TaxBehavior)
	assert.Equal(t, "exclusive", *params.TaxBehavior)
	assert.Equal(t, "tiered", *params.BillingScheme)
	assert.Nil(t, params.UnitAmount, "Stripe rejects a tiered price that also carries a flat amount")
}

// TestApplyPricing_ZeroAmountIsStillTaxExclusive — the free plan stays out of
// Stripe today, but a 0 EUR price must not be the one case that falls back to
// the account default and reintroduces the ambiguity.
func TestApplyPricing_ZeroAmountIsStillTaxExclusive(t *testing.T) {
	params := &stripe.PriceParams{}

	services.ApplyPricing(params, &models.SubscriptionPlan{PriceAmount: 0})

	require.NotNil(t, params.TaxBehavior)
	assert.Equal(t, "exclusive", *params.TaxBehavior)
}

// TestApplyPricing_EmptyTierListFallsBackToFlat — UseTieredPricing with no
// brackets must not produce a tiered price with no tiers, which Stripe rejects.
func TestApplyPricing_EmptyTierListFallsBackToFlat(t *testing.T) {
	params := &stripe.PriceParams{}

	services.ApplyPricing(params, &models.SubscriptionPlan{
		UseTieredPricing: true,
		PricingTiers:     nil,
		PriceAmount:      4900,
	})

	require.NotNil(t, params.TaxBehavior)
	assert.Equal(t, "exclusive", *params.TaxBehavior)
	assert.Equal(t, int64(4900), *params.UnitAmount)
	assert.Nil(t, params.BillingScheme)
}
