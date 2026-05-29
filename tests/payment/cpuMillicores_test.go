// tests/payment/cpuMillicores_test.go
//
// Regression coverage for the CPU budget unit switch from integer vCPU to
// integer millicores (mCPU). The wire unit is now mCPU throughout the
// Go code (1000 mCPU = 1 vCPU). XS = 500 mCPU because XS containers run
// at cpu_allowance=50% on tt-backend — under the old integer-vCPU unit,
// XS=1 over-counted by 2× and the budget rejected twice as few sessions
// as the user paid for.
//
// These tests pin two invariants:
//
//  1. XS occupies exactly 500 mCPU in the catalog. If someone edits the
//     catalog and accidentally reverts XS to 1 (the old vCPU number),
//     TestSizeCatalog_XSIs500Millicores fails loudly.
//
//  2. A 5000 mCPU plan (a "5 vCPU plan" in customer-facing language)
//     admits exactly ten XS sessions and rejects the eleventh. This is
//     the contract the unit switch was created to satisfy. The test
//     exercises the real QuotaService.CheckBudget against a real GORM
//     SQLite — no mocks, the assertion looks at the canonical decision.
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/catalog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSizeCatalog_XSIs500Millicores pins the post-switch catalog values
// so an accidental revert to the old integer-vCPU units fails here first
// (before any quota math test catches the cascading mistake).
func TestSizeCatalog_XSIs500Millicores(t *testing.T) {
	xs, ok := catalog.LookupSize("XS")
	require.True(t, ok, "XS must be present in the catalog")
	assert.Equal(t, 500, xs.CPU,
		"XS must be 500 mCPU (cpu_allowance=50% on tt-backend); 1 means a reverted unit switch")
	assert.Equal(t, 256, xs.MemoryMB, "XS RAM must remain 256 MiB")

	// Pin the rest of the catalog so the unit is uniform across all sizes.
	cases := []struct {
		key     string
		wantCPU int
	}{
		{"S", 1000},
		{"M", 2000},
		{"L", 4000},
		{"XL", 4000},
	}
	for _, tc := range cases {
		size, ok := catalog.LookupSize(tc.key)
		require.True(t, ok, "%s must be present in the catalog", tc.key)
		assert.Equalf(t, tc.wantCPU, size.CPU,
			"%s must be %d mCPU after the unit switch", tc.key, tc.wantCPU)
	}

	// LargestSize is the conservative fallback. It must follow the new unit too.
	assert.Equal(t, 4000, catalog.LargestSize().CPU,
		"LargestSize.CPU must be in mCPU (4000), not the old vCPU value (4)")
}

// TestCheckBudget_TenXSSessionsFitInFiveVCPUPlan exercises the end-to-end
// contract that motivated the switch:
//
//   - Plan MaxCPU=5000 (i.e., a "5 vCPU budget" in customer language).
//   - Each XS session reserves 500 mCPU.
//   - Ten XS sessions occupy exactly 10 × 500 = 5000 mCPU.
//   - The eleventh XS must be rejected.
//
// Before the unit switch, XS=1 / MaxCPU=5 meant only 5 sessions fit —
// half the capacity the user paid for. This test would have caught that
// regression.
func TestCheckBudget_TenXSSessionsFitInFiveVCPUPlan(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "u-mcpu-ten-xs"

	// 5 vCPU plan = 5000 mCPU. Memory generous so CPU is the binding axis.
	plan := budgetPlan(t, db, "FiveVCPUPlan", 5000, 65536, nil)

	xs, ok := catalog.LookupSize("XS")
	require.True(t, ok)

	svc := newQuotaSvc(t, db)

	// Seed 10 running XS sessions. Each one occupies xs.CPU mCPU.
	for i := 0; i < 10; i++ {
		insertTerminal(t, db, userID, nil, "running", "ephemeral", xs.CPU, xs.MemoryMB)
	}

	// At full budget, GetBudgetUsage must reflect 10 × 500 = 5000 mCPU.
	usedCPU, _, err := svc.GetBudgetUsage(userID, nil)
	require.NoError(t, err)
	assert.Equal(t, 5000, usedCPU,
		"10 XS sessions must consume exactly 5000 mCPU under the new unit")

	// An 11th XS must be rejected: 5000 used + 500 requested > 5000 cap.
	check, err := svc.CheckBudget(userID, nil, plan, xs.CPU, xs.MemoryMB)
	require.NoError(t, err)
	require.NotNil(t, check)
	assert.False(t, check.Allowed,
		"the 11th XS must exhaust the budget (10×500=5000 == cap, no headroom for another 500)")
	assert.Equal(t, "budget_cpu_exceeded", check.Reason,
		"rejection must name the CPU axis as the limiting factor")
}

// TestCheckBudget_NineXSSessionsLeaveRoomForOneMore is the symmetric
// happy-path counterpart: 9 XS sessions leave 500 mCPU of headroom, so
// the 10th must be admitted. Pins that the rejection above is not
// accidentally rejecting earlier than it should.
func TestCheckBudget_NineXSSessionsLeaveRoomForOneMore(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "u-mcpu-nine-xs"

	plan := budgetPlan(t, db, "FiveVCPUPlanHeadroom", 5000, 65536, nil)

	xs, ok := catalog.LookupSize("XS")
	require.True(t, ok)

	svc := newQuotaSvc(t, db)

	for i := 0; i < 9; i++ {
		insertTerminal(t, db, userID, nil, "running", "ephemeral", xs.CPU, xs.MemoryMB)
	}

	check, err := svc.CheckBudget(userID, nil, plan, xs.CPU, xs.MemoryMB)
	require.NoError(t, err)
	require.NotNil(t, check)
	assert.True(t, check.Allowed,
		"the 10th XS must fit: 9×500 used + 500 requested = 5000 == cap")
	assert.Empty(t, check.Reason)
}
