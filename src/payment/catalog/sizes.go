// Package catalog is the single source of truth for resource sizes used
// by the budget quota engine.
//
// tt-backend is the authoritative origin of the size catalog. At startup
// the in-process active catalog is hydrated from tt-backend's /sizes
// endpoint via Hydrate. The hardcoded fallback in this file is a
// cold-start safety net only: it lets ocf-core boot and serve budget
// decisions when tt-backend is unreachable, with values that match the
// historical seed. Per-key disagreements between the live data and the
// fallback are reported as Drift entries so they can be logged as
// warnings — that's the early-warning system for silent divergence
// between the two services.
//
// The catalog is intentionally a leaf package: depended on by quota,
// terminal, and scenario code but importing nothing OCF-specific. This
// keeps the dependency graph acyclic.
package catalog

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// MachineSize describes the CPU + memory footprint of one size class.
//
// CPU is expressed in integer millicores (mCPU): 1000 mCPU = 1 vCPU. The
// granularity matches what the terminal backend actually enforces via
// Incus cpu_allowance — XS containers run at 50% of one vCPU, i.e.
// 500 mCPU, NOT 1 vCPU. Keeping the unit as mCPU lets the budget engine
// price XS correctly (500 mCPU per session) so a "5 vCPU" budget admits
// the right number of XS sessions instead of half as many. Display
// conversion to fractional vCPU is the frontend's responsibility.
type MachineSize struct {
	CPU      int // millicores (mCPU); 1000 = 1 vCPU
	MemoryMB int // MiB
}

// SourceSize is the raw, unparsed shape emitted by tt-backend's /sizes
// endpoint. Hydrate translates these into MachineSize entries.
type SourceSize struct {
	Key          string
	CPU          int    // raw cpuset count (NOT used in mCPU math)
	CPUAllowance string // e.g. "50%", "200%"
	Memory       string // e.g. "256MiB", "1GiB"
	SortOrder    int
}

// Drift describes one disagreement between the hardcoded fallback and
// the live tt-backend catalog observed during a Hydrate call.
type Drift struct {
	Key      string
	Fallback MachineSize
	Live     MachineSize
	Reason   string
}

// fallbackCatalog is the cold-start safety net: the values that were
// historically hardcoded here. Only lowercase keys — case handling lives
// in LookupSize.
var fallbackCatalog = map[string]MachineSize{
	"xs": {CPU: 500, MemoryMB: 256},
	"s":  {CPU: 1000, MemoryMB: 512},
	"m":  {CPU: 2000, MemoryMB: 1024},
	"l":  {CPU: 4000, MemoryMB: 2048},
	"xl": {CPU: 4000, MemoryMB: 4096},
}

// fallbackKeys is the canonical lowercase key order for the fallback.
var fallbackKeys = []string{"xs", "s", "m", "l", "xl"}

// fallbackLargest matches the largest fallback entry (xl).
var fallbackLargest = MachineSize{CPU: 4000, MemoryMB: 4096}

// Active state — read by every consumer, written by Hydrate. Guarded by
// mu. Initialised at package-init time to the fallback values so
// pre-hydration callers (and tt-backend-unreachable boots) still get
// correct answers.
var (
	mu            sync.RWMutex
	active        map[string]MachineSize
	activeKeys    []string
	activeLargest MachineSize
)

func init() {
	resetActiveForTesting()
}

// resetActiveForTesting restores the active catalog to the hardcoded
// fallback. Exposed in production code (lowercase first letter — package
// private) but only called from tests and from package init. Tests use
// it to start each Hydrate-sensitive case from a known baseline.
func resetActiveForTesting() {
	mu.Lock()
	defer mu.Unlock()
	active = make(map[string]MachineSize, len(fallbackCatalog))
	for k, v := range fallbackCatalog {
		active[k] = v
	}
	activeKeys = append([]string(nil), fallbackKeys...)
	activeLargest = fallbackLargest
}

// LookupSize returns the CPU/memory footprint for a size key
// (case-insensitive), and whether the key matched an active catalog
// entry.
func LookupSize(key string) (MachineSize, bool) {
	normalized := strings.ToLower(strings.TrimSpace(key))
	mu.RLock()
	defer mu.RUnlock()
	size, ok := active[normalized]
	return size, ok
}

// LargestSize returns the worst-case footprint of the active catalog,
// used as a conservative fallback when a caller cannot resolve a
// specific size (e.g. RAM estimation before the user has picked one).
// CPU is in millicores (mCPU).
func LargestSize() MachineSize {
	mu.RLock()
	defer mu.RUnlock()
	return activeLargest
}

// CanonicalSizeKeys returns a copy of the lowercase size keys in
// monotonically increasing footprint order, for callers that need to
// iterate the catalog deterministically (e.g. ComputeRemainingBySize).
func CanonicalSizeKeys() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, len(activeKeys))
	copy(out, activeKeys)
	return out
}

// MCPUFor returns the effective CPU budget cost in millicores for a
// size key, or 0 if the key is not in the active catalog. Convenience
// wrapper around LookupSize for the common stamping case.
func MCPUFor(key string) int {
	size, ok := LookupSize(key)
	if !ok {
		return 0
	}
	return size.CPU
}

// Hydrate replaces the active catalog with the values parsed from the
// given sources, and returns the drift entries observed against the
// hardcoded fallback. Sources that fail to parse are reported as a
// "parse error" drift entry and excluded from the new active catalog.
//
// Drift ordering: source-order entries first ("new size unknown to
// fallback", "cpu changed", "memory changed", "cpu and memory changed",
// "parse error: ..."), then "fallback key missing from live catalog"
// entries appended at the end in alphabetical order by key.
func Hydrate(sources []SourceSize) []Drift {
	// Phase 1: parse + compute drifts before mutating the active state.
	parsed := make(map[string]MachineSize, len(sources))
	orderedKeys := make([]string, 0, len(sources))
	keyToSource := make(map[string]SourceSize, len(sources))
	drifts := make([]Drift, 0)

	for _, src := range sources {
		key := strings.ToLower(strings.TrimSpace(src.Key))
		if key == "" {
			continue
		}

		cpuPercent, err := parseAllowance(src.CPUAllowance)
		if err != nil {
			drifts = append(drifts, Drift{
				Key:      key,
				Fallback: fallbackCatalog[key], // zero-value if absent
				Live:     MachineSize{},
				Reason:   fmt.Sprintf("parse error: %v", err),
			})
			continue
		}
		memMB, err := parseMemory(src.Memory)
		if err != nil {
			drifts = append(drifts, Drift{
				Key:      key,
				Fallback: fallbackCatalog[key],
				Live:     MachineSize{},
				Reason:   fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		live := MachineSize{CPU: cpuPercent * 10, MemoryMB: memMB}
		parsed[key] = live
		orderedKeys = append(orderedKeys, key)
		keyToSource[key] = src

		fallback, present := fallbackCatalog[key]
		switch {
		case !present:
			drifts = append(drifts, Drift{
				Key:      key,
				Fallback: MachineSize{},
				Live:     live,
				Reason:   "new size unknown to fallback",
			})
		case live.CPU != fallback.CPU && live.MemoryMB != fallback.MemoryMB:
			drifts = append(drifts, Drift{
				Key:      key,
				Fallback: fallback,
				Live:     live,
				Reason:   "cpu and memory changed",
			})
		case live.CPU != fallback.CPU:
			drifts = append(drifts, Drift{
				Key:      key,
				Fallback: fallback,
				Live:     live,
				Reason:   "cpu changed",
			})
		case live.MemoryMB != fallback.MemoryMB:
			drifts = append(drifts, Drift{
				Key:      key,
				Fallback: fallback,
				Live:     live,
				Reason:   "memory changed",
			})
		}
	}

	// Fallback keys not in the live catalog — appended at the end,
	// alphabetical order for deterministic output.
	missing := make([]string, 0)
	for key := range fallbackCatalog {
		if _, ok := parsed[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	for _, key := range missing {
		drifts = append(drifts, Drift{
			Key:      key,
			Fallback: fallbackCatalog[key],
			Live:     MachineSize{},
			Reason:   "fallback key missing from live catalog",
		})
	}

	// Phase 2: build new active state.
	sort.SliceStable(orderedKeys, func(i, j int) bool {
		return keyToSource[orderedKeys[i]].SortOrder < keyToSource[orderedKeys[j]].SortOrder
	})

	newLargest := MachineSize{}
	for _, size := range parsed {
		if size.MemoryMB > newLargest.MemoryMB ||
			(size.MemoryMB == newLargest.MemoryMB && size.CPU > newLargest.CPU) {
			newLargest = size
		}
	}

	mu.Lock()
	active = parsed
	activeKeys = orderedKeys
	activeLargest = newLargest
	mu.Unlock()

	return drifts
}

// parseAllowance parses a tt-backend cpu_allowance string ("50%", "200%")
// into its integer percent prefix. Empty input, missing trailing "%",
// or a non-numeric prefix all produce an error.
func parseAllowance(s string) (int, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, fmt.Errorf("empty allowance")
	}
	if !strings.HasSuffix(trimmed, "%") {
		return 0, fmt.Errorf("allowance missing trailing %%: %q", trimmed)
	}
	digits := strings.TrimSuffix(trimmed, "%")
	if digits == "" {
		return 0, fmt.Errorf("allowance has no numeric prefix: %q", trimmed)
	}
	pct, err := strconv.Atoi(digits)
	if err != nil {
		return 0, fmt.Errorf("allowance not numeric: %q", trimmed)
	}
	return pct, nil
}

// parseMemory parses a tt-backend memory string ("256MiB", "1GiB", "1GB")
// into MiB. Accepts MiB/MB (1:1) and GiB/GB (×1024). Empty input,
// missing unit, or a non-numeric prefix all produce an error.
func parseMemory(s string) (int, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, fmt.Errorf("empty memory")
	}

	var digits string
	var multiplier int
	switch {
	case strings.HasSuffix(trimmed, "MiB"):
		digits = strings.TrimSuffix(trimmed, "MiB")
		multiplier = 1
	case strings.HasSuffix(trimmed, "GiB"):
		digits = strings.TrimSuffix(trimmed, "GiB")
		multiplier = 1024
	case strings.HasSuffix(trimmed, "MB"):
		digits = strings.TrimSuffix(trimmed, "MB")
		multiplier = 1
	case strings.HasSuffix(trimmed, "GB"):
		digits = strings.TrimSuffix(trimmed, "GB")
		multiplier = 1024
	default:
		return 0, fmt.Errorf("memory missing unit suffix (MiB/MB/GiB/GB): %q", trimmed)
	}

	digits = strings.TrimSpace(digits)
	if digits == "" {
		return 0, fmt.Errorf("memory has no numeric prefix: %q", trimmed)
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, fmt.Errorf("memory not numeric: %q", trimmed)
	}
	return n * multiplier, nil
}
