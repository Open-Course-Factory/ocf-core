// tests/payment/stripeMetadata_test.go
//
// Tests for Stripe Product metadata serialization/deserialization tied to
// SubscriptionPlan budget fields.
//
// The metadata payload carries plan_id + the CPU/RAM budget caps. Empty
// metadata round-trips to zeros (which the budget engine interprets as
// unlimited).
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
)

func TestStripeMetadata_WriteBudgetPlan_EmitsCPUAndMemoryKeys(t *testing.T) {
	// MaxCPU is in millicores (mCPU): 8000 mCPU = 8 vCPU.
	plan := &models.SubscriptionPlan{
		Name:        "Budget Pro",
		MaxCPU:      8000,
		MaxMemoryMB: 4096,
	}

	metadata := services.BuildPlanProductMetadata(plan)

	assert.Equal(t, "8000", metadata["max_cpu"], "max_cpu must be the stringified MaxCPU value (mCPU)")
	assert.Equal(t, "4096", metadata["max_memory_mb"], "max_memory_mb must be the stringified MaxMemoryMB value")
	assert.NotEmpty(t, metadata["plan_id"], "plan_id must be emitted so importers can reconcile")
}

func TestStripeMetadata_WriteZeroBudget_EmitsZeros(t *testing.T) {
	plan := &models.SubscriptionPlan{Name: "Unlimited"}

	metadata := services.BuildPlanProductMetadata(plan)

	assert.Equal(t, "0", metadata["max_cpu"])
	assert.Equal(t, "0", metadata["max_memory_mb"])
}

func TestStripeMetadata_ReadBudgetMetadata_ParsesCPUAndMemory(t *testing.T) {
	// max_cpu is stored as stringified mCPU (1000 mCPU = 1 vCPU).
	metadata := map[string]string{
		"max_cpu":       "8000",
		"max_memory_mb": "4096",
	}

	parsed := services.ParsePlanProductMetadata(metadata)

	assert.Equal(t, 8000, parsed.MaxCPU)
	assert.Equal(t, 4096, parsed.MaxMemoryMB)
}

func TestStripeMetadata_ReadEmpty_DefaultsToZero(t *testing.T) {
	parsed := services.ParsePlanProductMetadata(map[string]string{})

	assert.Equal(t, 0, parsed.MaxCPU)
	assert.Equal(t, 0, parsed.MaxMemoryMB)
}

func TestStripeMetadata_ReadGarbageInts_DefaultsToZero(t *testing.T) {
	// Stripe metadata is always string — defensively handle malformed ints.
	metadata := map[string]string{
		"max_cpu":       "not-a-number",
		"max_memory_mb": "",
	}

	parsed := services.ParsePlanProductMetadata(metadata)

	assert.Equal(t, 0, parsed.MaxCPU)
	assert.Equal(t, 0, parsed.MaxMemoryMB)
}
