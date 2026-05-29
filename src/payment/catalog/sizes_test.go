// Tests for the size catalog hydration API.
//
// Red phase of issue #340: ocf-core's hardcoded sizeCatalog becomes a
// cold-start fallback. Hydrate(sources []SourceSize) lets tt-backend become
// the single source of truth at startup. These tests pin the contract that
// the implementer must satisfy.
package catalog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// setup resets the active catalog before each Hydrate-sensitive test so
// each test starts from a known fallback state. Implementer must expose
// resetActiveForTesting() in the production package (package-private,
// shared via the catalog package boundary in _test.go).
func setup(t *testing.T) {
	t.Helper()
	resetActiveForTesting()
}

// canonicalSources returns the 5 size records as tt-backend's dbSeedSizes
// would emit them: cpu cpuset count, cpu_allowance string, memory string.
func canonicalSources() []SourceSize {
	return []SourceSize{
		{Key: "xs", CPU: 1, CPUAllowance: "50%", Memory: "256MiB", SortOrder: 1},
		{Key: "s", CPU: 1, CPUAllowance: "100%", Memory: "512MiB", SortOrder: 2},
		{Key: "m", CPU: 2, CPUAllowance: "200%", Memory: "1GiB", SortOrder: 3},
		{Key: "l", CPU: 4, CPUAllowance: "400%", Memory: "2GiB", SortOrder: 4},
		{Key: "xl", CPU: 4, CPUAllowance: "400%", Memory: "4GiB", SortOrder: 5},
	}
}

// --- Test 1: parseAllowance ---------------------------------------------------

func Test_parseAllowance(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"50 percent", "50%", 50, false},
		{"100 percent", "100%", 100, false},
		{"200 percent", "200%", 200, false},
		{"400 percent", "400%", 400, false},
		{"with surrounding spaces", "  50%  ", 50, false},
		{"empty string", "", 0, true},
		{"non-numeric", "abc", 0, true},
		{"missing percent sign", "50", 0, true},
		{"bare percent", "%", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAllowance(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Equal(t, 0, got)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// --- Test 2: parseMemory ------------------------------------------------------

func Test_parseMemory(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"256 MiB", "256MiB", 256, false},
		{"512 MiB", "512MiB", 512, false},
		{"1 GiB", "1GiB", 1024, false},
		{"2 GiB", "2GiB", 2048, false},
		{"4 GiB", "4GiB", 4096, false},
		{"with surrounding spaces", "  1GiB  ", 1024, false},
		{"MB lenient", "256MB", 256, false},
		{"GB lenient", "1GB", 1024, false},
		{"empty string", "", 0, true},
		{"non-numeric", "abc", 0, true},
		{"missing unit", "256", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMemory(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Equal(t, 0, got)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// --- Test 3: Hydrate happy path, no drift -------------------------------------

func Test_Hydrate_HappyPath_NoDrift(t *testing.T) {
	setup(t)

	drift := Hydrate(canonicalSources())
	assert.Empty(t, drift, "feeding canonical sources verbatim must produce no drift")

	// Verify every canonical size resolves to the expected MachineSize.
	expected := map[string]MachineSize{
		"xs": {CPU: 500, MemoryMB: 256},
		"s":  {CPU: 1000, MemoryMB: 512},
		"m":  {CPU: 2000, MemoryMB: 1024},
		"l":  {CPU: 4000, MemoryMB: 2048},
		"xl": {CPU: 4000, MemoryMB: 4096},
	}
	for key, want := range expected {
		got, ok := LookupSize(key)
		assert.True(t, ok, "LookupSize(%q) must succeed after hydrate", key)
		assert.Equal(t, want, got, "LookupSize(%q) mismatch", key)
	}

	// Canonical keys ordered by SortOrder.
	assert.Equal(t, []string{"xs", "s", "m", "l", "xl"}, CanonicalSizeKeys())
}

// --- Test 4: Hydrate allowance change reports drift ---------------------------

func Test_Hydrate_AllowanceChange_ReportsDrift(t *testing.T) {
	setup(t)

	sources := canonicalSources()
	// xs is index 0; bump allowance from 50% to 100% — expected mCPU goes 500 -> 1000.
	sources[0].CPUAllowance = "100%"

	drift := Hydrate(sources)
	assert.Len(t, drift, 1, "exactly one drift entry for xs allowance change")
	d := drift[0]
	assert.Equal(t, "xs", d.Key)
	assert.Equal(t, MachineSize{CPU: 500, MemoryMB: 256}, d.Fallback)
	assert.Equal(t, MachineSize{CPU: 1000, MemoryMB: 256}, d.Live)
	assert.Contains(t, strings.ToLower(d.Reason), "cpu", "Reason must mention cpu")

	// Live wins after hydrate.
	got, ok := LookupSize("xs")
	assert.True(t, ok)
	assert.Equal(t, MachineSize{CPU: 1000, MemoryMB: 256}, got)
}

// --- Test 5: Hydrate memory change reports drift ------------------------------

func Test_Hydrate_MemoryChange_ReportsDrift(t *testing.T) {
	setup(t)

	sources := canonicalSources()
	sources[0].Memory = "512MiB" // xs: 256 -> 512

	drift := Hydrate(sources)
	assert.Len(t, drift, 1)
	d := drift[0]
	assert.Equal(t, "xs", d.Key)
	assert.Equal(t, MachineSize{CPU: 500, MemoryMB: 256}, d.Fallback)
	assert.Equal(t, MachineSize{CPU: 500, MemoryMB: 512}, d.Live)
	assert.Contains(t, strings.ToLower(d.Reason), "memory", "Reason must mention memory")
}

// --- Test 6: Hydrate new key reports drift ------------------------------------

func Test_Hydrate_NewKey_ReportsDrift(t *testing.T) {
	setup(t)

	sources := append(canonicalSources(), SourceSize{
		Key: "xxl", CPU: 8, CPUAllowance: "800%", Memory: "8GiB", SortOrder: 6,
	})

	drift := Hydrate(sources)
	assert.Len(t, drift, 1, "exactly one drift entry for the new xxl size")
	d := drift[0]
	assert.Equal(t, "xxl", d.Key)
	assert.Equal(t, MachineSize{CPU: 0, MemoryMB: 0}, d.Fallback, "Fallback is zero-value for unknown key")
	assert.Equal(t, MachineSize{CPU: 8000, MemoryMB: 8192}, d.Live)
	reasonLower := strings.ToLower(d.Reason)
	assert.True(t,
		strings.Contains(reasonLower, "new") || strings.Contains(reasonLower, "unknown"),
		"Reason must mention 'new' or 'unknown', got %q", d.Reason,
	)

	// New key is now resolvable.
	got, ok := LookupSize("xxl")
	assert.True(t, ok)
	assert.Equal(t, MachineSize{CPU: 8000, MemoryMB: 8192}, got)

	// Canonical keys include xxl, placed by SortOrder.
	keys := CanonicalSizeKeys()
	assert.Contains(t, keys, "xxl")
	assert.Equal(t, "xxl", keys[len(keys)-1], "xxl has highest SortOrder, so last")
}

// --- Test 7: Hydrate missing key reports drift --------------------------------

func Test_Hydrate_MissingKey_ReportsDrift(t *testing.T) {
	setup(t)

	// Omit xl (the last canonical entry).
	full := canonicalSources()
	sources := full[:4]

	drift := Hydrate(sources)
	assert.Len(t, drift, 1, "exactly one drift entry for the missing xl size")
	d := drift[0]
	assert.Equal(t, "xl", d.Key)
	assert.Equal(t, MachineSize{CPU: 4000, MemoryMB: 4096}, d.Fallback)
	assert.Equal(t, MachineSize{CPU: 0, MemoryMB: 0}, d.Live, "Live is zero-value when key absent from live data")
	assert.Contains(t, strings.ToLower(d.Reason), "missing", "Reason must mention missing")

	// xl no longer resolves.
	_, ok := LookupSize("xl")
	assert.False(t, ok, "xl must not be in the active catalog after hydrate without it")
}

// --- Test 8: Hydrate is idempotent / safe to call twice -----------------------

func Test_Hydrate_Idempotent(t *testing.T) {
	setup(t)

	sources := canonicalSources()
	// First call: applies hydration.
	assert.NotPanics(t, func() { _ = Hydrate(sources) })
	// Second call: must not panic. Whether it re-applies or no-ops is impl detail,
	// but the active catalog must remain consistent.
	assert.NotPanics(t, func() { _ = Hydrate(sources) })

	got, ok := LookupSize("xs")
	assert.True(t, ok)
	assert.Equal(t, MachineSize{CPU: 500, MemoryMB: 256}, got)
}

// --- Test 9: LookupSize case-insensitive after Hydrate ------------------------

func Test_LookupSize_CaseInsensitive_AfterHydrate(t *testing.T) {
	setup(t)
	_ = Hydrate(canonicalSources())

	want := MachineSize{CPU: 500, MemoryMB: 256}
	for _, key := range []string{"xs", "XS", "Xs", " xs ", " XS "} {
		got, ok := LookupSize(key)
		assert.True(t, ok, "LookupSize(%q) must succeed", key)
		assert.Equal(t, want, got, "LookupSize(%q) mismatch", key)
	}
}

// --- Test 10: MCPUFor ---------------------------------------------------------

func Test_MCPUFor(t *testing.T) {
	setup(t)
	_ = Hydrate(canonicalSources())

	assert.Equal(t, 500, MCPUFor("xs"))
	assert.Equal(t, 2000, MCPUFor("M"), "case-insensitive")
	assert.Equal(t, 0, MCPUFor("unknown"))
}

// --- Test 11: Hydrate empty input wipes the active catalog -------------------

// Test_Hydrate_EmptyInput_ReportsAllFallbackKeysMissing pins the leaf-package
// contract: Hydrate is a pure transformation. An empty input wipes the active
// catalog and reports every fallback key as missing. Callers that want to
// treat "empty live data" as a failure must guard upstream (see
// HydrateSizeCatalog in src/initialization).
func Test_Hydrate_EmptyInput_ReportsAllFallbackKeysMissing(t *testing.T) {
	resetActiveForTesting()
	drifts := Hydrate(nil)
	assert.Len(t, drifts, 5, "every fallback key should be reported as missing")

	_, ok := LookupSize("xs")
	assert.False(t, ok, "active catalog is empty after Hydrate(nil); caller is responsible for guarding")
}

// --- Test 12: LargestSize after Hydrate ---------------------------------------

func Test_LargestSize_AfterHydrate(t *testing.T) {
	setup(t)
	_ = Hydrate(canonicalSources())
	assert.Equal(t, MachineSize{CPU: 4000, MemoryMB: 4096}, LargestSize())

	setup(t) // reset to ensure clean state before hydrating with xxl
	sources := append(canonicalSources(), SourceSize{
		Key: "xxl", CPU: 8, CPUAllowance: "800%", Memory: "8GiB", SortOrder: 6,
	})
	_ = Hydrate(sources)
	assert.Equal(t, MachineSize{CPU: 8000, MemoryMB: 8192}, LargestSize())
}
