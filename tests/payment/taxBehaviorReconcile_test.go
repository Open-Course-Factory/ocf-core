// tests/payment/taxBehaviorReconcile_test.go
//
// Tax behaviour is the half of a price that decides who pays the VAT, and it is
// the half nothing could repair.
//
//   - The update DTO carried no tax_behavior and no price_amount, so the
//     catalogue reconcile brought a stale plan's description and priority back in
//     line and left its money exactly as it was.
//   - The Stripe sync compared amount, currency and interval, never behaviour, so
//     a price carrying the wrong one was never migrated — and Stripe accepts the
//     answer once per price, so "not migrated" means "wrong forever".
//
// Both halves are pinned here: the fields exist and survive a round trip, and an
// unusable value is refused rather than quietly resolved to a default.
package payment_tests

import (
	"encoding/json"
	"testing"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reconcile sends its patch as JSON; a field the DTO does not declare is
// dropped in silence, which is exactly how the price and the tax behaviour went
// missing while the rest of the plan updated.
func TestUpdateSubscriptionPlanInput_CarriesPriceAndTaxBehavior(t *testing.T) {
	var input dto.UpdateSubscriptionPlanInput
	require.NoError(t, json.Unmarshal(
		[]byte(`{"price_amount": 1990, "tax_behavior": "inclusive"}`), &input))

	require.NotNil(t, input.PriceAmount,
		"a patch that states a price must not be dropped — the reconcile sends this")
	assert.Equal(t, int64(1990), *input.PriceAmount)
	assert.Equal(t, "inclusive", input.TaxBehavior)
}

// Omitting the amount must leave it alone rather than rewrite it to zero, which
// is why the field is a pointer.
func TestUpdateSubscriptionPlanInput_OmittedPriceIsNotZero(t *testing.T) {
	var input dto.UpdateSubscriptionPlanInput
	require.NoError(t, json.Unmarshal([]byte(`{"priority": 40}`), &input))

	assert.Nil(t, input.PriceAmount,
		"an absent price must stay absent — a zero here would make every partial "+
			"update a free plan")
	assert.Empty(t, input.TaxBehavior)
}

func TestPlanValidation_AcceptsTheTwoRealTaxBehaviors(t *testing.T) {
	for _, behavior := range []string{"inclusive", "exclusive"} {
		t.Run(behavior, func(t *testing.T) {
			assert.NoError(t, execPlanValidation(hooks.BeforeCreate,
				&models.SubscriptionPlan{TaxBehavior: behavior}))
			assert.NoError(t, execPlanValidation(hooks.BeforeUpdate,
				map[string]any{"tax_behavior": behavior}))
		})
	}
}

// An unknown value used to be accepted and then resolved to "exclusive" by
// taxBehaviorOf — turning an announced TTC price into a net one and billing the
// VAT twice over. Refuse it while it is still a request.
func TestPlanValidation_RejectsAnUnusableTaxBehavior(t *testing.T) {
	// The offending field travels in Details, not in the message — assert on it
	// so this cannot be satisfied by some other validation failing instead.
	rejectedField := func(t *testing.T, err error) string {
		t.Helper()
		var entityErr *entityErrors.EntityError
		require.ErrorAs(t, err, &entityErr)
		field, _ := entityErr.Details["field"].(string)
		return field
	}

	err := execPlanValidation(hooks.BeforeCreate,
		&models.SubscriptionPlan{TaxBehavior: "ttc"})
	require.Error(t, err, "an unknown tax behaviour must not reach a Stripe price")
	assert.Equal(t, "tax_behavior", rejectedField(t, err))

	err = execPlanValidation(hooks.BeforeUpdate, map[string]any{"tax_behavior": "ttc"})
	require.Error(t, err, "the update path must refuse it too")
	assert.Equal(t, "tax_behavior", rejectedField(t, err))
}

// A partial update that says nothing about tax is not making a claim about it.
func TestPlanValidation_SilenceOnTaxBehaviorIsNotAnError(t *testing.T) {
	assert.NoError(t, execPlanValidation(hooks.BeforeUpdate,
		map[string]any{"priority": 40}))

	// The empty string is the legacy "never said", which taxBehaviorOf already
	// answers for; rejecting it here would make every pre-existing plan
	// unupdatable.
	assert.NoError(t, execPlanValidation(hooks.BeforeCreate,
		&models.SubscriptionPlan{TaxBehavior: ""}))
	assert.NoError(t, execPlanValidation(hooks.BeforeUpdate,
		map[string]any{"tax_behavior": ""}))
}

// The generic PATCH path decodes the pointer-field DTO via mapstructure and
// leaves pointers in the patch map, so the hook has to read that shape too.
func TestPlanValidation_ReadsTaxBehaviorThroughAPointer(t *testing.T) {
	bad := "ttc"
	require.Error(t, execPlanValidation(hooks.BeforeUpdate,
		map[string]any{"tax_behavior": &bad}))

	good := "inclusive"
	assert.NoError(t, execPlanValidation(hooks.BeforeUpdate,
		map[string]any{"tax_behavior": &good}))

	var absent *string
	assert.NoError(t, execPlanValidation(hooks.BeforeUpdate,
		map[string]any{"tax_behavior": absent}),
		"a nil pointer is an absent field, not an empty claim")
}
