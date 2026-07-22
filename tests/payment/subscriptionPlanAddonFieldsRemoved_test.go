// tests/payment/subscriptionPlanAddonFieldsRemoved_test.go
//
// TASK 1 (owner: "delete, we'll see later"): remove the three dead add-on price
// ID fields from models.SubscriptionPlan — AddonNetworkPriceID /
// AddonStoragePriceID / AddonTerminalPriceID. They have ZERO consumers: no DTO
// (SubscriptionPlanOutput has no addon_* field), no converter, no Stripe path,
// no other test references them. They never surfaced through the API, so there
// is NOTHING to pin on the DTO/Output round-trip side — the only user-observable
// change is (1) the model fields disappear and (2) the reflection round-trip
// sweep no longer needs its addon_* exclusion entries.
//
// This is a pure deletion; the honest RED is structural, driven by reflection
// over the real model type + the sweep's own exclusion map (both live in this
// package). Both assertions FAIL today (fields + exclusions present) and pass
// once the fields and the matching exclusions are removed together — proving the
// exclusions became unnecessary because the fields are gone (see the sibling
// TestSubscriptionPlanConverter_AllModelFieldsRoundTrip, which must stay green).
package payment_tests

import (
	"reflect"
	"testing"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
)

// addonJSONKeys are the json keys of the three dead add-on price ID fields.
var addonJSONKeys = []string{
	"addon_network_price_id",
	"addon_storage_price_id",
	"addon_terminal_price_id",
}

// TestSubscriptionPlan_AddonPriceIDFields_Removed asserts the model no longer
// declares any of the three add-on price ID fields. modelJSONKey is reused from
// subscriptionPlanConverterRoundTrip_test.go (same package). RED today: the
// fields exist on the struct.
func TestSubscriptionPlan_AddonPriceIDFields_Removed(t *testing.T) {
	pt := reflect.TypeOf(models.SubscriptionPlan{})

	present := map[string]bool{}
	for i := 0; i < pt.NumField(); i++ {
		if key := modelJSONKey(pt.Field(i)); key != "" {
			present[key] = true
		}
	}

	for _, key := range addonJSONKeys {
		assert.False(t, present[key],
			"model field with json %q must be deleted — it is dead (no DTO/converter/Stripe/test consumer)", key)
	}
}

// TestSubscriptionPlanOutput_HasNoAddonFields is a belt-and-braces guard that
// the public output contract never carried the addon_* keys (it never did — so
// removing the model fields has no output-side effect). Kept so a future
// re-introduction is caught. This one passes today; it documents the N/A.
func TestSubscriptionPlanOutput_HasNoAddonFields(t *testing.T) {
	ot := reflect.TypeOf(dto.SubscriptionPlanOutput{})

	present := map[string]bool{}
	for i := 0; i < ot.NumField(); i++ {
		if key := modelJSONKey(ot.Field(i)); key != "" {
			present[key] = true
		}
	}
	for _, key := range addonJSONKeys {
		assert.False(t, present[key],
			"SubscriptionPlanOutput must not expose %q (it never did — the removal is model-only)", key)
	}
}

// TestRoundTripExclusions_NoLongerNeedAddonKeys asserts the round-trip sweep's
// exclusion map (outputContractExclusions, declared in
// subscriptionPlanConverterRoundTrip_test.go) no longer lists the addon_* keys.
// Once the model fields are gone the sweep never walks those keys, so their
// exclusions are dead and must be removed with the fields. RED today: the three
// entries are present.
func TestRoundTripExclusions_NoLongerNeedAddonKeys(t *testing.T) {
	for _, key := range addonJSONKeys {
		_, excluded := outputContractExclusions[key]
		assert.False(t, excluded,
			"outputContractExclusions[%q] is dead once the model field is deleted — remove it with the field", key)
	}
}
