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
//
// Calling Hydrate with an empty input wipes the active catalog and
// returns drift entries for every fallback key. Callers that treat
// "tt-backend returned nothing" as a failure mode must guard upstream;
// see HydrateSizeCatalog in src/initialization.
func Hydrate(sources []SourceSize) []Drift {
	parsed, keyToSource, drifts := parseAll(sources)
	drifts = append(drifts, missingFallbackDrifts(parsed)...)
	swapActive(parsed, orderedBySort(keyToSource), largestOf(parsed))
	return drifts
}

// parseAll walks the input sources, normalizes keys, parses each
// source into a MachineSize, and classifies drift against the
// fallback catalog. Parse failures are reported as drift entries and
// excluded from the parsed map.
func parseAll(sources []SourceSize) (map[string]MachineSize, map[string]SourceSize, []Drift) {
	parsed := make(map[string]MachineSize, len(sources))
	keyToSource := make(map[string]SourceSize, len(sources))
	drifts := make([]Drift, 0)

	for _, src := range sources {
		key := strings.ToLower(strings.TrimSpace(src.Key))
		if key == "" {
			continue
		}

		live, err := parseSource(src)
		if err != nil {
			drifts = append(drifts, Drift{
				Key:      key,
				Fallback: fallbackCatalog[key], // zero-value if absent
				Live:     MachineSize{},
				Reason:   fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		parsed[key] = live
		keyToSource[key] = src

		if d, ok := classifyDrift(key, live); ok {
			drifts = append(drifts, d)
		}
	}

	return parsed, keyToSource, drifts
}

// parseSource turns one raw SourceSize into a MachineSize, returning
// the first parse error encountered.
func parseSource(src SourceSize) (MachineSize, error) {
	cpuPercent, err := parseAllowance(src.CPUAllowance)
	if err != nil {
		return MachineSize{}, err
	}
	memMB, err := parseMemory(src.Memory)
	if err != nil {
		return MachineSize{}, err
	}
	return MachineSize{CPU: cpuPercent * 10, MemoryMB: memMB}, nil
}

// classifyDrift reports the drift entry for one live size against the
// fallback catalog, or (_, false) if there is no drift. A key absent
// from the fallback yields a "new size unknown to fallback" entry; a
// key present but with different values is classified by which axis
// changed.
func classifyDrift(key string, live MachineSize) (Drift, bool) {
	fallback, present := fallbackCatalog[key]
	if !present {
		return Drift{
			Key:      key,
			Fallback: MachineSize{},
			Live:     live,
			Reason:   "new size unknown to fallback",
		}, true
	}

	cpuChanged := live.CPU != fallback.CPU
	memChanged := live.MemoryMB != fallback.MemoryMB
	reason := driftReason(cpuChanged, memChanged)
	if reason == "" {
		return Drift{}, false
	}
	return Drift{
		Key:      key,
		Fallback: fallback,
		Live:     live,
		Reason:   reason,
	}, true
}

// driftReason maps the (cpuChanged, memChanged) tuple to its reason
// string. Empty string means no drift. Order-independent — replaces
// the previous switch with its hidden "both must come first" trap.
func driftReason(cpuChanged, memChanged bool) string {
	switch {
	case cpuChanged && memChanged:
		return "cpu and memory changed"
	case cpuChanged:
		return "cpu changed"
	case memChanged:
		return "memory changed"
	}
	return ""
}

// missingFallbackDrifts returns one drift entry per fallback key that
// the live catalog omits, in alphabetical order for deterministic
// output.
func missingFallbackDrifts(parsed map[string]MachineSize) []Drift {
	missing := make([]string, 0)
	for key := range fallbackCatalog {
		if _, ok := parsed[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)

	out := make([]Drift, 0, len(missing))
	for _, key := range missing {
		out = append(out, Drift{
			Key:      key,
			Fallback: fallbackCatalog[key],
			Live:     MachineSize{},
			Reason:   "fallback key missing from live catalog",
		})
	}
	return out
}

// orderedBySort returns the parsed keys sorted by their source
// SortOrder, the canonical iteration order for the active catalog.
func orderedBySort(keyToSource map[string]SourceSize) []string {
	keys := make([]string, 0, len(keyToSource))
	for k := range keyToSource {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return keyToSource[keys[i]].SortOrder < keyToSource[keys[j]].SortOrder
	})
	return keys
}

// largestOf returns the worst-case footprint of the parsed catalog:
// max MemoryMB, with CPU as the tiebreaker.
func largestOf(parsed map[string]MachineSize) MachineSize {
	largest := MachineSize{}
	for _, size := range parsed {
		if size.MemoryMB > largest.MemoryMB ||
			(size.MemoryMB == largest.MemoryMB && size.CPU > largest.CPU) {
			largest = size
		}
	}
	return largest
}

// swapActive replaces the active catalog state under the write lock.
func swapActive(parsed map[string]MachineSize, orderedKeys []string, largest MachineSize) {
	mu.Lock()
	defer mu.Unlock()
	active = parsed
	activeKeys = orderedKeys
	activeLargest = largest
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
