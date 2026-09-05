package payment_tests

import (
	"testing"

	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
)

// The "0 means unlimited" convention is written down once, on the model that
// carries MaxCPU / MaxMemoryMB. These tests pin its boundary so a later edit
// cannot quietly narrow it back to `== 0` — the narrowing that had already
// happened at several call sites, where a negative budget read as unlimited to
// the quota engine and as a finite (negative) cap elsewhere.
func TestIsUnlimitedBudget_Boundary(t *testing.T) {
	assert.True(t, models.IsUnlimitedBudget(0), "0 is the documented unlimited sentinel")
	assert.True(t, models.IsUnlimitedBudget(-1), "a negative budget is not a finite cap")
	assert.False(t, models.IsUnlimitedBudget(1))
	assert.False(t, models.IsUnlimitedBudget(24000))
}

func TestSubscriptionPlan_AxisUnlimitedPredicates(t *testing.T) {
	capped := models.SubscriptionPlan{MaxCPU: 24000, MaxMemoryMB: 12288}
	assert.False(t, capped.IsCPUUnlimited())
	assert.False(t, capped.IsMemoryUnlimited())

	unlimited := models.SubscriptionPlan{}
	assert.True(t, unlimited.IsCPUUnlimited())
	assert.True(t, unlimited.IsMemoryUnlimited())
}

// HasBudgetCap replaced two De Morgan-equivalent spellings of the same
// question that lived in the same file. The mixed cases are the ones those
// spellings were most likely to disagree on.
func TestSubscriptionPlan_HasBudgetCap(t *testing.T) {
	cases := []struct {
		name        string
		cpu, memory int
		want        bool
	}{
		{"both capped", 24000, 12288, true},
		{"both unlimited", 0, 0, false},
		{"cpu capped only", 1000, 0, true},
		{"memory capped only", 0, 512, true},
		{"negative reads as unlimited", -1, -1, false},
		{"negative cpu, capped memory", -1, 512, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := models.SubscriptionPlan{MaxCPU: tc.cpu, MaxMemoryMB: tc.memory}
			assert.Equal(t, tc.want, plan.HasBudgetCap())
		})
	}
}
