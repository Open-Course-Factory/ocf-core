// tests/payment/checkoutInputHardening_test.go
//
// RED-phase tests for issue #376 / MR !278 — checkout input hardening.
//
// Premise-check results (both gaps confirmed on current main):
//  1. Quantity cap: BulkPurchaseInput.Quantity, CreateBulkCheckoutSessionInput.
//     Quantity, and UpdateBatchQuantityInput.NewQuantity all carry
//     `binding:"required,min=1"` but NO max, so an arbitrarily large quantity
//     passes validation. Desired: max=1000.
//  2. Non-catalog rejection: CreateCheckoutSession / CreateBulkCheckoutSession
//     check only `!plan.IsActive` (stripeService.go:264/:412); an active plan
//     with IsCatalog=false (custom/unlisted) is accepted for self-service
//     checkout. Desired: reject it. AdminAssignSubscription is a SEPARATE path
//     (subscriptionService.AdminAssignSubscription — no Stripe, no IsActive/
//     IsCatalog check) so it is unaffected (noted for the dev; not gated here).
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------------
// Item 1 — bulk quantity cap (max=1000) at the binding-validation seam.
// binding.Validator is exactly what gin's ShouldBindJSON runs, reading the
// `binding:"..."` struct tags. All required fields are set so the ONLY possible
// failure is the (missing) max rule.
// -----------------------------------------------------------------------------

func TestBulkPurchaseInput_QuantityCappedAt1000(t *testing.T) {
	require.NoError(t, binding.Validator.ValidateStruct(&dto.BulkPurchaseInput{
		SubscriptionPlanID: uuid.New(), Quantity: 50,
	}), "a normal quantity must validate")
	assert.NoError(t, binding.Validator.ValidateStruct(&dto.BulkPurchaseInput{
		SubscriptionPlanID: uuid.New(), Quantity: 1000,
	}), "1000 (the cap boundary) must be valid")

	assert.Error(t, binding.Validator.ValidateStruct(&dto.BulkPurchaseInput{
		SubscriptionPlanID: uuid.New(), Quantity: 5000,
	}), "CAP: BulkPurchaseInput.Quantity must be capped (binding max=1000) — today it has "+
		"no max, so 5000 validates and a request could provision thousands of licenses.")
}

func TestCreateBulkCheckoutSessionInput_QuantityCappedAt1000(t *testing.T) {
	require.NoError(t, binding.Validator.ValidateStruct(&dto.CreateBulkCheckoutSessionInput{
		SubscriptionPlanID: uuid.New(), Quantity: 50, SuccessURL: "https://s", CancelURL: "https://c",
	}), "a normal quantity must validate")

	assert.Error(t, binding.Validator.ValidateStruct(&dto.CreateBulkCheckoutSessionInput{
		SubscriptionPlanID: uuid.New(), Quantity: 5000, SuccessURL: "https://s", CancelURL: "https://c",
	}), "CAP: CreateBulkCheckoutSessionInput.Quantity must be capped at max=1000.")
}

func TestUpdateBatchQuantityInput_QuantityCappedAt1000(t *testing.T) {
	require.NoError(t, binding.Validator.ValidateStruct(&dto.UpdateBatchQuantityInput{
		NewQuantity: 50,
	}), "a normal quantity must validate")

	assert.Error(t, binding.Validator.ValidateStruct(&dto.UpdateBatchQuantityInput{
		NewQuantity: 5000,
	}), "CAP: UpdateBatchQuantityInput.NewQuantity must be capped at max=1000.")
}

// -----------------------------------------------------------------------------
// Item 2 — reject non-catalog plans in self-service checkout.
// Driven through the real StripeService + fake Stripe backend (captures the
// checkout-session POST body) + fake Casdoor.
// -----------------------------------------------------------------------------

func seedCheckoutPlan(t *testing.T, name string, isCatalog bool) *models.SubscriptionPlan {
	t.Helper()
	priceID := "price_hard_" + uuid.NewString()
	return &models.SubscriptionPlan{
		Name: name, PriceAmount: 1999, Currency: "eur", BillingInterval: "month",
		StripePriceID: &priceID, IsActive: true, IsCatalog: isCatalog,
	}
}

// A non-catalog plan (active, IsCatalog=false) must be rejected in self-service
// checkout, with NO Stripe session created.
func TestStripeService_CreateCheckoutSession_RejectsNonCatalogPlan(t *testing.T) {
	db := freshTestDB(t)
	cap := installTaxFormCapturingStripe(t)
	installFakeCasdoor(t, "nc@example.com", "NC User")
	svc := services.NewStripeService(db)

	plan := seedCheckoutPlan(t, "Custom Unlisted", false)
	// GORM's Create skips zero-value bools on a field with gorm:"default:true", so
	// is_catalog persists TRUE at Create; force it false via a follow-up Update
	// (the pattern subscriptionPlan_catalog_test.go actually uses).
	require.NoError(t, db.Create(plan).Error)
	require.NoError(t, db.Model(plan).Update("is_catalog", false).Error)

	_, err := svc.CreateCheckoutSession("user_nc_"+uuid.NewString(), dto.CreateCheckoutSessionInput{
		SubscriptionPlanID: plan.ID, SuccessURL: "https://app.test/s", CancelURL: "https://app.test/c",
	}, nil)

	assert.Error(t, err,
		"CATALOG: a non-catalog plan (IsCatalog=false) must be rejected in self-service "+
			"checkout — today CreateCheckoutSession only checks IsActive, so a custom/"+
			"unlisted plan is purchasable by anyone who knows its id.")
	assert.Empty(t, cap.checkoutSessionForm(),
		"no Stripe checkout session may be created for a rejected non-catalog plan")
}

func TestStripeService_CreateBulkCheckoutSession_RejectsNonCatalogPlan(t *testing.T) {
	db := freshTestDB(t)
	cap := installTaxFormCapturingStripe(t)
	installFakeCasdoor(t, "ncbulk@example.com", "NC Bulk User")
	svc := services.NewStripeService(db)

	plan := seedCheckoutPlan(t, "Custom Unlisted Bulk", false)
	// Force is_catalog=false via Update (see above).
	require.NoError(t, db.Create(plan).Error)
	require.NoError(t, db.Model(plan).Update("is_catalog", false).Error)

	_, err := svc.CreateBulkCheckoutSession("user_ncbulk_"+uuid.NewString(), dto.CreateBulkCheckoutSessionInput{
		SubscriptionPlanID: plan.ID, Quantity: 5, SuccessURL: "https://app.test/s", CancelURL: "https://app.test/c",
	})

	assert.Error(t, err, "CATALOG: bulk checkout must also reject a non-catalog plan.")
	assert.Empty(t, cap.checkoutSessionForm(),
		"no Stripe checkout session may be created for a rejected non-catalog plan (bulk)")
}

// Guard: a normal CATALOG plan still checks out fine (the rejection must be
// narrow — IsCatalog=false only). GREEN today and after.
func TestStripeService_CreateCheckoutSession_CatalogPlanSucceeds(t *testing.T) {
	db := freshTestDB(t)
	cap := installTaxFormCapturingStripe(t)
	installFakeCasdoor(t, "cat@example.com", "Catalog User")
	svc := services.NewStripeService(db)

	plan := seedCheckoutPlan(t, "Standard Catalog", true)
	require.NoError(t, db.Create(plan).Error)

	out, err := svc.CreateCheckoutSession("user_cat_"+uuid.NewString(), dto.CreateCheckoutSessionInput{
		SubscriptionPlanID: plan.ID, SuccessURL: "https://app.test/s", CancelURL: "https://app.test/c",
	}, nil)

	require.NoError(t, err, "a catalog plan must still check out")
	require.NotNil(t, out)
	assert.NotEmpty(t, cap.checkoutSessionForm(), "a catalog plan must create a Stripe checkout session")
}
