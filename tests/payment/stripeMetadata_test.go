// tests/payment/stripeMetadata_test.go
//
// Tests for dual-mode (legacy + new) Stripe Product metadata
// serialization/deserialization tied to SubscriptionPlan budget fields
// (MR-CORE-7).
//
// Writes always emit BOTH the legacy `max_concurrent_terminals` key AND the
// three new budget keys (`max_cpu`, `max_memory_mb`, `quota_model`). Reads
// prefer the new keys and fall back to the legacy key when only the legacy
// key is present. Empty metadata → safe defaults (count-model, zeros).
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
)

func TestStripeMetadata_WriteBudgetPlan_IncludesNewKeys(t *testing.T) {
	plan := &models.SubscriptionPlan{
		Name:                   "Budget Pro",
		MaxConcurrentTerminals: 4,
		MaxCPU:                 8,
		MaxMemoryMB:            4096,
		QuotaModel:             "budget",
	}

	metadata := services.BuildPlanProductMetadata(plan)

	// New keys present with correct values
	assert.Equal(t, "8", metadata["max_cpu"], "max_cpu must be stringified MaxCPU")
	assert.Equal(t, "4096", metadata["max_memory_mb"], "max_memory_mb must be stringified MaxMemoryMB")
	assert.Equal(t, "budget", metadata["quota_model"], "quota_model must reflect plan.QuotaModel")

	// Legacy key still emitted (dual-mode write)
	assert.Equal(t, "4", metadata["max_concurrent_terminals"], "legacy max_concurrent_terminals must still be written")
}

func TestStripeMetadata_WriteLegacyPlan_NewKeysHaveDefaults(t *testing.T) {
	plan := &models.SubscriptionPlan{
		Name:                   "Count Legacy",
		MaxConcurrentTerminals: 2,
		MaxCPU:                 0,
		MaxMemoryMB:            0,
		QuotaModel:             "count",
	}

	metadata := services.BuildPlanProductMetadata(plan)

	// Always-emit policy: zeros are still written explicitly
	assert.Equal(t, "0", metadata["max_cpu"])
	assert.Equal(t, "0", metadata["max_memory_mb"])
	assert.Equal(t, "count", metadata["quota_model"])
	assert.Equal(t, "2", metadata["max_concurrent_terminals"])
}

func TestStripeMetadata_WriteEmptyQuotaModel_DefaultsToCount(t *testing.T) {
	// A plan with QuotaModel unset (zero-value "") should serialize as
	// "count" — the field has a DB default of "count" but in-memory zero
	// values shouldn't leak into Stripe.
	plan := &models.SubscriptionPlan{
		Name: "Unset Quota Model",
	}

	metadata := services.BuildPlanProductMetadata(plan)

	assert.Equal(t, "count", metadata["quota_model"], "empty QuotaModel must default to 'count'")
}

func TestStripeMetadata_ReadBudgetMetadata_PrefersNewKeys(t *testing.T) {
	// Both old and new keys present with conflicting values — new keys win.
	metadata := map[string]string{
		"max_concurrent_terminals": "2",
		"max_cpu":                  "8",
		"max_memory_mb":            "4096",
		"quota_model":              "budget",
	}

	parsed := services.ParsePlanProductMetadata(metadata)

	assert.Equal(t, 8, parsed.MaxCPU)
	assert.Equal(t, 4096, parsed.MaxMemoryMB)
	assert.Equal(t, "budget", parsed.QuotaModel)
	// Legacy field still surfaced — caller decides what to do with it.
	assert.Equal(t, 2, parsed.MaxConcurrentTerminals)
}

func TestStripeMetadata_ReadLegacyMetadata_FallsBackToOldKeys(t *testing.T) {
	// Only legacy key present → new keys default to zero, model is "count".
	metadata := map[string]string{
		"max_concurrent_terminals": "3",
	}

	parsed := services.ParsePlanProductMetadata(metadata)

	assert.Equal(t, 0, parsed.MaxCPU, "missing max_cpu → default 0")
	assert.Equal(t, 0, parsed.MaxMemoryMB, "missing max_memory_mb → default 0")
	assert.Equal(t, "count", parsed.QuotaModel, "missing quota_model → default 'count'")
	assert.Equal(t, 3, parsed.MaxConcurrentTerminals, "legacy max_concurrent_terminals preserved")
}

func TestStripeMetadata_ReadEmpty_DefaultsToCount(t *testing.T) {
	parsed := services.ParsePlanProductMetadata(map[string]string{})

	assert.Equal(t, 0, parsed.MaxCPU)
	assert.Equal(t, 0, parsed.MaxMemoryMB)
	assert.Equal(t, "count", parsed.QuotaModel)
	assert.Equal(t, 0, parsed.MaxConcurrentTerminals)
}

func TestStripeMetadata_ReadGarbageInts_DefaultsToZero(t *testing.T) {
	// Stripe metadata is always string — defensively handle malformed ints.
	metadata := map[string]string{
		"max_cpu":                  "not-a-number",
		"max_memory_mb":            "",
		"max_concurrent_terminals": "abc",
		"quota_model":              "budget",
	}

	parsed := services.ParsePlanProductMetadata(metadata)

	assert.Equal(t, 0, parsed.MaxCPU)
	assert.Equal(t, 0, parsed.MaxMemoryMB)
	assert.Equal(t, 0, parsed.MaxConcurrentTerminals)
	assert.Equal(t, "budget", parsed.QuotaModel)
}
