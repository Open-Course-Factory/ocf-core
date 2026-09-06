// tests/terminalTrainer/terminalKeyBudget_test.go
//
// Tests for the plan ceiling -> Terminal Trainer per-key budget conversion.
//
// The two systems count CPU differently and this is the seam between them:
//
//   - ocf-core prices CPU in mCPU, allowance-aware. Size "xs" runs at a 50%
//     CPU allowance and therefore costs 500 mCPU.
//   - tt-backend's max_cpu_total counts WHOLE CPUs from its size catalog,
//     where "xs" is cpu: 1.
//
// So a budget expressed in mCPU cannot map exactly onto tt-backend's units.
// The conversion rounds UP to whole vCPU, which is the fail-safe direction:
// it can only ever grant more headroom on the tt-backend side, never less,
// so the tt-backend cap stays a backstop and never becomes the binding
// constraint that silently contradicts the plan the learner was sold.
package terminalTrainer_tests

import (
	"testing"

	"soli/formations/src/payment/catalog"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudgetForTerminalKey_RoundsPartialCPUUp(t *testing.T) {
	// 1500 mCPU = 1.5 vCPU. Truncating would hand the learner 1 vCPU and make
	// tt-backend refuse a session the plan allows.
	cpu, mem := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
		MaxCPU:      1500,
		MaxMemoryMB: 2048,
	})

	require.NotNil(t, cpu)
	require.NotNil(t, mem)
	assert.Equal(t, int64(2), *cpu, "1.5 vCPU must round up to 2")
	assert.Equal(t, int64(2048), *mem)
}

func TestBudgetForTerminalKey_ExactCPUIsNotInflated(t *testing.T) {
	cpu, _ := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
		MaxCPU:      24000,
		MaxMemoryMB: 12288,
	})

	require.NotNil(t, cpu)
	assert.Equal(t, int64(24), *cpu)
}

func TestBudgetForTerminalKey_SubVCPUCeilingStillGrantsOne(t *testing.T) {
	// A 500 mCPU learner plan (one XS session) must not convert to a 0 budget:
	// tt-backend rejects 0 outright, and it would mean "may launch nothing".
	cpu, mem := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
		MaxCPU:      500,
		MaxMemoryMB: 256,
	})

	require.NotNil(t, cpu)
	assert.Equal(t, int64(1), *cpu)
	require.NotNil(t, mem)
	assert.Equal(t, int64(256), *mem)
}

// A non-positive budget on an axis sends no cap for that axis rather than a
// zero: tt-backend rejects an explicit 0 outright, so forwarding one would turn
// a bad row into a failed key provisioning. Plan validation refuses to create
// such a budget, so this guards rows that predate it and the user who holds no
// plan in any context, whose ceiling is zero on both axes. The per-key budget
// is defence in depth; ocf-core's own gate is what refuses them.
func TestBudgetForTerminalKey_NonPositiveAxisSendsNoCap(t *testing.T) {
	cases := []struct {
		name        string
		cpu, memory int
		wantCPU     bool
		wantMem     bool
	}{
		{"both zero", 0, 0, false, false},
		{"zero cpu only", 0, 4096, false, true},
		{"zero memory only", 8000, 0, true, false},
		{"both negative", -1, -1, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cpu, mem := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
				MaxCPU:      tc.cpu,
				MaxMemoryMB: tc.memory,
			})
			assert.Equal(t, tc.wantCPU, cpu != nil, "cpu cap presence")
			assert.Equal(t, tc.wantMem, mem != nil, "memory cap presence")
		})
	}
}

// TestBudgetForTerminalKey_UsesCatalogCPUScale: the conversion divides by the
// same millicore scale the size catalog is priced in, so one vCPU of plan
// budget is one whole CPU on the key — not a second, locally defined 1000.
func TestBudgetForTerminalKey_UsesCatalogCPUScale(t *testing.T) {
	cpu, _ := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
		MaxCPU: 3 * catalog.MilliCPUPerVCPU,
	})
	require.NotNil(t, cpu)
	assert.Equal(t, int64(3), *cpu)
}
