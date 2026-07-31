// tests/payment/orgSubscriptionEmbeddedPlan_test.go
//
// The admin organizations panel crashed with
// "RangeError: invalid currency code in NumberFormat()".
//
// Cause: two still-active organization subscriptions point at a subscription
// plan that was soft-deleted on 2026-07-22. GORM's Preload honours the soft
// delete, so the association silently stays at its zero value — and the
// handler then converted that zero value into a plan output, emitting
// {name: "", currency: "", price_amount: 0} as if it were a real plan.
//
// The frontend already typed `subscription_plan?` as optional and guards with
// `if (!plan) return null`, but an empty struct still serialises to an object,
// which is truthy — so the guard never fired and "" reached Intl.NumberFormat.
//
// The defect is that "no plan" was not representable. A free plan and a plan
// that failed to load produced byte-identical JSON. Emitting nil restores the
// distinction, and every consumer's existing absence check starts working.
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/models"
	controller "soli/formations/src/payment/routes"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedPlanOutput_UnloadedAssociationIsNil is the bug: a preload that
// skipped a soft-deleted row leaves the zero value behind, and it must not be
// dressed up as a plan.
func TestEmbeddedPlanOutput_UnloadedAssociationIsNil(t *testing.T) {
	var unloaded models.SubscriptionPlan // exactly what GORM leaves behind

	got := controller.EmbeddedPlanOutput(&unloaded)

	assert.Nil(t, got, "an association that did not load is not a plan — emitting one "+
		"with currency \"\" is what crashed the admin organizations panel")
}

// TestEmbeddedPlanOutput_NilIsNil — the explicit no-plan case.
func TestEmbeddedPlanOutput_NilIsNil(t *testing.T) {
	assert.Nil(t, controller.EmbeddedPlanOutput(nil))
}

// TestEmbeddedPlanOutput_RealPlanIsPreserved guards the other direction: the
// absence check must not swallow plans that did load.
func TestEmbeddedPlanOutput_RealPlanIsPreserved(t *testing.T) {
	plan := &models.SubscriptionPlan{
		Name:            "Formateur",
		PriceAmount:     1990,
		Currency:        "eur",
		BillingInterval: "month",
	}
	plan.ID = uuid.New()

	got := controller.EmbeddedPlanOutput(plan)

	require.NotNil(t, got)
	assert.Equal(t, "Formateur", got.Name)
	assert.Equal(t, "eur", got.Currency)
	assert.Equal(t, int64(1990), got.PriceAmount)
}

// TestEmbeddedPlanOutput_FreePlanIsStillAPlan — a 0 EUR plan is real and must
// survive, which is precisely why absence cannot be inferred from the amount.
func TestEmbeddedPlanOutput_FreePlanIsStillAPlan(t *testing.T) {
	plan := &models.SubscriptionPlan{Name: "Trial", PriceAmount: 0, Currency: "eur"}
	plan.ID = uuid.New()

	got := controller.EmbeddedPlanOutput(plan)

	require.NotNil(t, got, "a free plan is a plan; only a missing ID means it never loaded")
	assert.Equal(t, "eur", got.Currency)
}
