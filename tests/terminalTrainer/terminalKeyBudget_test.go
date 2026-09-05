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

	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/terminalTrainer/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudgetForTerminalKey_RoundsPartialCPUUp(t *testing.T) {
	// 1500 mCPU = 1.5 vCPU. Truncating would hand the learner 1 vCPU and make
	// tt-backend refuse a session the plan allows.
	cpu, mem := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
		MaxCPU:         1500,
		MaxMemoryMB:    2048,
		HasEntitlement: true,
	})

	require.NotNil(t, cpu)
	require.NotNil(t, mem)
	assert.Equal(t, int64(2), *cpu, "1.5 vCPU must round up to 2")
	assert.Equal(t, int64(2048), *mem)
}

func TestBudgetForTerminalKey_ExactCPUIsNotInflated(t *testing.T) {
	cpu, _ := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
		MaxCPU:         24000,
		MaxMemoryMB:    12288,
		HasEntitlement: true,
	})

	require.NotNil(t, cpu)
	assert.Equal(t, int64(24), *cpu)
}

func TestBudgetForTerminalKey_SubVCPUCeilingStillGrantsOne(t *testing.T) {
	// A 500 mCPU learner plan (one XS session) must not convert to a 0 budget:
	// tt-backend rejects 0 outright, and it would mean "may launch nothing".
	cpu, mem := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
		MaxCPU:         500,
		MaxMemoryMB:    256,
		HasEntitlement: true,
	})

	require.NotNil(t, cpu)
	assert.Equal(t, int64(1), *cpu)
	require.NotNil(t, mem)
	assert.Equal(t, int64(256), *mem)
}

func TestBudgetForTerminalKey_UnlimitedSendsNoCap(t *testing.T) {
	// 0 on a plan axis means unlimited. tt-backend expresses "no budget" as a
	// NULL column, which its request DTO models as an omitted pointer.
	cpu, mem := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
		MaxCPU:         0,
		MaxMemoryMB:    0,
		HasEntitlement: true,
	})

	assert.Nil(t, cpu, "unlimited CPU must send no cap, not a zero")
	assert.Nil(t, mem, "unlimited RAM must send no cap, not a zero")
}

func TestBudgetForTerminalKey_UnlimitedOnOneAxisOnly(t *testing.T) {
	cpu, mem := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
		MaxCPU:         0,
		MaxMemoryMB:    4096,
		HasEntitlement: true,
	})

	assert.Nil(t, cpu)
	require.NotNil(t, mem)
	assert.Equal(t, int64(4096), *mem)
}

func TestBudgetForTerminalKey_NoEntitlementSendsNoCap(t *testing.T) {
	// A user with no plan in any context. tt-backend cannot express a zero
	// budget, so nothing is sent and ocf-core's own gate remains the thing
	// that refuses them — the per-key budget is defense-in-depth, not the
	// primary entitlement check.
	cpu, mem := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
		HasEntitlement: false,
	})

	assert.Nil(t, cpu)
	assert.Nil(t, mem)
}

// A negative budget is nonsense data that plan validation now refuses at the
// door. Should one reach here anyway — a row predating the validation — it must
// not be sent to tt-backend, which rejects a non-positive budget outright.
func TestBudgetForTerminalKey_NegativeCeilingSendsNoCap(t *testing.T) {
	cpu, mem := services.BudgetForTerminalKey(paymentServices.UserBudgetCeiling{
		MaxCPU:         -1,
		MaxMemoryMB:    -1,
		HasEntitlement: true,
	})

	assert.Nil(t, cpu)
	assert.Nil(t, mem)
}
