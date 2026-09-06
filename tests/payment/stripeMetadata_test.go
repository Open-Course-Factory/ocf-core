// tests/payment/stripeMetadata_test.go
//
// Tests for Stripe Product metadata serialization/deserialization tied to
// SubscriptionPlan budget fields.
//
// The metadata payload carries plan_id + the CPU/RAM budget caps. A key the
// product does not carry states nothing: the importer decides what that means,
// and it never means a zero budget.
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// The writer states what the plan holds, zero included: a zero budget must be
// visible on the Stripe side, where the importer will refuse it, rather than
// silently dropped.
func TestStripeMetadata_WriteZeroBudget_EmitsZeros(t *testing.T) {
	plan := &models.SubscriptionPlan{Name: "Accidental zero"}

	metadata := services.BuildPlanProductMetadata(plan)

	assert.Equal(t, "0", metadata["max_cpu"])
	assert.Equal(t, "0", metadata["max_memory_mb"])
}

func TestStripeMetadata_ReadBudgetMetadata_ParsesCPUAndMemory(t *testing.T) {
	metadata := map[string]string{
		"max_cpu":       "8000",
		"max_memory_mb": "4096",
	}

	parsed := services.ParsePlanProductMetadata(metadata)

	require.NotNil(t, parsed.MaxCPU)
	require.NotNil(t, parsed.MaxMemoryMB)
	assert.Equal(t, 8000, *parsed.MaxCPU)
	assert.Equal(t, 4096, *parsed.MaxMemoryMB)
}

// An absent key states nothing. It must not read as a zero: the importer
// treats "not stated" as "leave the plan alone", and a zero would be a budget
// that grants nothing.
func TestStripeMetadata_ReadEmpty_StatesNoAxis(t *testing.T) {
	parsed := services.ParsePlanProductMetadata(map[string]string{})

	assert.Nil(t, parsed.MaxCPU)
	assert.Nil(t, parsed.MaxMemoryMB)
}

func TestStripeMetadata_ReadGarbageInts_StatesNoAxis(t *testing.T) {
	metadata := map[string]string{
		"max_cpu":       "not-a-number",
		"max_memory_mb": "",
	}

	parsed := services.ParsePlanProductMetadata(metadata)

	assert.Nil(t, parsed.MaxCPU)
	assert.Nil(t, parsed.MaxMemoryMB)
}

// A stated zero is stated. The importer must see it and refuse it, not treat
// it as absent.
func TestStripeMetadata_ReadStatedZero_IsStated(t *testing.T) {
	parsed := services.ParsePlanProductMetadata(map[string]string{"max_cpu": "0"})

	require.NotNil(t, parsed.MaxCPU)
	assert.Equal(t, 0, *parsed.MaxCPU)
	assert.Nil(t, parsed.MaxMemoryMB)
}
