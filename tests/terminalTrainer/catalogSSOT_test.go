// Catalog SSOT tests — assert that the former mirrors of the size catalog
// in capacityService.go and terminalTrainerService.go now derive their
// values from backfill.LookupSize.
//
// Pre-cleanup, three places re-encoded the size→RAM mapping in ocf-core:
//   - backfill.sizeCatalog (canonical CPU+RAM source)
//   - capacityService.machineSizeToRAM (GB)
//   - terminalTrainerService.go inline map (GB, for bulk pre-flight RAM)
//
// The cleanup unifies the latter two on backfill.LookupSize. These tests
// pin the behavioural parity: if a future maintainer reintroduces a
// hardcoded literal that drifts from backfill, the test fails.
package terminalTrainer_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/payment/backfill"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"
)

// TestCapacityService_UsesBackfillLookup_ForRequestedSize — for each
// canonical size, the RAM estimate consumed by EvaluateLaunchCapacity
// matches backfill.LookupSize(key).MemoryMB / 1024.
//
// The test computes the expected post-launch availability from the
// catalog value, then asserts the resulting CapacityStatus boundary
// (OK vs Critical) matches what backfill predicts. A drift in either
// direction (smaller RAM → too lax; larger RAM → too strict) flips
// the boundary and fails the assertion.
func TestCapacityService_UsesBackfillLookup_ForRequestedSize(t *testing.T) {
	// Plan allows every canonical size to remove the planAllowsSize gate.
	plan := &paymentModels.SubscriptionPlan{
		AllowedMachineSizes: []string{"XS", "S", "M", "L", "XL"},
	}

	for _, key := range []string{"XS", "S", "M", "L", "XL"} {
		size, ok := backfill.LookupSize(key)
		require.True(t, ok, "backfill must know %s", key)
		expectedRAMGB := float64(size.MemoryMB) / 1024.0

		// Build metrics where avail = expectedRAM + tiny epsilon ABOVE
		// reserve (total = 100GB, reserve = 5GB). After launch availability
		// must remain ≥ reserve → OK. If capacityService had a stale
		// (larger) RAM estimate it would over-deduct and trip Critical.
		availGB := expectedRAMGB + 6.0 // 1GB margin above reserve
		// total such that available = avail and reserve = avail-6:
		// total = avail / (1 - usedPct/100) → set usedPct so total = avail+something
		// Pick total = 100GB regardless of avail: usedPct = (1 - avail/100)*100
		usedPct := (1.0 - availGB/100.0) * 100.0
		metrics := &dto.ServerMetricsResponse{
			RAMAvailableGB: availGB,
			RAMPercent:     usedPct,
		}

		got := services.EvaluateLaunchCapacity(plan, key, metrics)
		assert.NotEqual(t, services.CapacityStatusCritical, got.Status,
			"size %s: launch should fit (avail=%.2f GB, required=%.2f GB) — capacityService likely deducts a stale RAM value",
			key, availGB, expectedRAMGB)

		// Conversely: just below the required value → Critical.
		tightAvailGB := expectedRAMGB - 0.01 // less RAM than the catalog says we need
		if tightAvailGB <= 0 {
			continue // XS at 0.25GB minus epsilon stays positive; this is defensive
		}
		tightUsedPct := (1.0 - tightAvailGB/100.0) * 100.0
		tightMetrics := &dto.ServerMetricsResponse{
			RAMAvailableGB: tightAvailGB,
			RAMPercent:     tightUsedPct,
		}
		tightGot := services.EvaluateLaunchCapacity(plan, key, tightMetrics)
		assert.Equal(t, services.CapacityStatusCritical, tightGot.Status,
			"size %s: launch should be rejected (avail=%.2f GB, required=%.2f GB) — capacityService likely deducts a stale RAM value",
			key, tightAvailGB, expectedRAMGB)
	}
}

// TestCapacityService_UsesBackfillLookup_ForPlanMax — when no size is
// requested, EvaluateLaunchCapacity falls back to the plan's largest
// allowed size. The estimate must derive from backfill.LookupSize, not
// a stale local literal.
func TestCapacityService_UsesBackfillLookup_ForPlanMax(t *testing.T) {
	// Plan allows up to L → expected required RAM = backfill["L"].MemoryMB / 1024.
	l, ok := backfill.LookupSize("L")
	require.True(t, ok)
	expectedRAMGB := float64(l.MemoryMB) / 1024.0

	plan := &paymentModels.SubscriptionPlan{
		AllowedMachineSizes: []string{"XS", "S", "M", "L"},
	}

	// Total ≈ 100GB, avail just above (expected + reserve) → OK.
	availGB := expectedRAMGB + 6.0
	usedPct := (1.0 - availGB/100.0) * 100.0
	metrics := &dto.ServerMetricsResponse{
		RAMAvailableGB: availGB,
		RAMPercent:     usedPct,
	}
	got := services.EvaluateLaunchCapacity(plan, "", metrics)
	assert.NotEqual(t, services.CapacityStatusCritical, got.Status,
		"plan max L should fit (required=%.2fGB, avail=%.2fGB) — fallback path likely uses a stale literal",
		expectedRAMGB, availGB)

	// And tight: avail just under expected → Critical.
	tightAvailGB := expectedRAMGB - 0.01
	tightUsedPct := (1.0 - tightAvailGB/100.0) * 100.0
	tightMetrics := &dto.ServerMetricsResponse{
		RAMAvailableGB: tightAvailGB,
		RAMPercent:     tightUsedPct,
	}
	tightGot := services.EvaluateLaunchCapacity(plan, "", tightMetrics)
	assert.Equal(t, services.CapacityStatusCritical, tightGot.Status,
		"plan max L should be rejected when avail < required (required=%.2fGB, avail=%.2fGB)",
		expectedRAMGB, tightAvailGB)
}

// TestEstimatePerTerminalRAMGB_UsesBackfillLookup — exercises the
// extracted helper that replaces the inline catalog literal in
// terminalTrainerService.go (bulk RAM pre-flight). Behaviour parity:
// the helper must agree with backfill.LookupSize for every canonical
// key, and must return 1.0 (M-equivalent) for the "all" sentinel.
func TestEstimatePerTerminalRAMGB_UsesBackfillLookup(t *testing.T) {
	for _, key := range []string{"XS", "S", "M", "L", "XL"} {
		size, ok := backfill.LookupSize(key)
		require.True(t, ok)
		expected := float64(size.MemoryMB) / 1024.0
		got := services.EstimatePerTerminalRAMGB([]string{key})
		assert.Equal(t, expected, got, "size %s: expected %.2f GB from backfill, got %.2f", key, expected, got)
	}

	t.Run("all_sentinel_returns_M_equivalent", func(t *testing.T) {
		// "all" sentinel preserves the legacy 1GB estimate (matches M).
		// Use the M entry from the catalog so a future catalog change to
		// M flows through automatically.
		m, ok := backfill.LookupSize("M")
		require.True(t, ok)
		expected := float64(m.MemoryMB) / 1024.0
		got := services.EstimatePerTerminalRAMGB([]string{"all"})
		assert.Equal(t, expected, got, "'all' sentinel should equal M's RAM (legacy default)")
	})

	t.Run("empty_allowlist_returns_S_default", func(t *testing.T) {
		// Empty/unset allowlist preserves the legacy 0.5GB default (S).
		s, ok := backfill.LookupSize("S")
		require.True(t, ok)
		expected := float64(s.MemoryMB) / 1024.0
		got := services.EstimatePerTerminalRAMGB(nil)
		assert.Equal(t, expected, got, "empty allowlist should equal S's RAM (legacy default)")
	})

	t.Run("max_of_multiple_sizes", func(t *testing.T) {
		// When the plan allows several sizes the estimate is the max.
		l, ok := backfill.LookupSize("L")
		require.True(t, ok)
		expected := float64(l.MemoryMB) / 1024.0
		got := services.EstimatePerTerminalRAMGB([]string{"XS", "S", "M", "L"})
		assert.Equal(t, expected, got, "multi-size allowlist should pick max → L")
	})
}
