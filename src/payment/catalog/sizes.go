// Package catalog is the single source of truth for resource sizes used
// by the budget quota engine.
//
// MIRROR OF tt-backend/backend/db.go `dbSeedSizes` — keep in sync. If a
// new size is added on the terminal backend it must be mirrored here so
// the budget engine produces accurate caps.
//
// The catalog is intentionally a leaf package: depended on by quota,
// terminal, and scenario code but importing nothing OCF-specific. This
// keeps the dependency graph acyclic.
package catalog

import "strings"

// MachineSize describes the CPU + memory footprint of one size class.
type MachineSize struct {
	CPU      int
	MemoryMB int
}

// sizeCatalog is keyed by both the canonical uppercase code and the
// lowercase variant that some external callers (Stripe, legacy fixtures)
// emit. LookupSize handles both transparently.
var sizeCatalog = map[string]MachineSize{
	"XS": {CPU: 1, MemoryMB: 256},
	"xs": {CPU: 1, MemoryMB: 256},
	"S":  {CPU: 1, MemoryMB: 512},
	"s":  {CPU: 1, MemoryMB: 512},
	"M":  {CPU: 2, MemoryMB: 1024},
	"m":  {CPU: 2, MemoryMB: 1024},
	"L":  {CPU: 4, MemoryMB: 2048},
	"l":  {CPU: 4, MemoryMB: 2048},
	"XL": {CPU: 4, MemoryMB: 4096},
	"xl": {CPU: 4, MemoryMB: 4096},
}

// LargestSize is the worst-case footprint used as a conservative fallback
// when a caller cannot resolve a specific size (e.g. RAM estimation
// before the user has picked one). Matches the catalog's largest entry.
var LargestSize = MachineSize{CPU: 4, MemoryMB: 4096}

// CanonicalSizeKeys is the list of canonical (lowercase) size keys in
// monotonically increasing footprint order. Exposed so callers can iterate
// the catalog deterministically (e.g. for ComputeRemainingBySize).
var CanonicalSizeKeys = []string{"xs", "s", "m", "l", "xl"}

// LookupSize returns the CPU/memory footprint for a size key
// (case-insensitive), and whether the key matched a catalog entry.
func LookupSize(key string) (MachineSize, bool) {
	if size, ok := sizeCatalog[key]; ok {
		return size, true
	}
	if size, ok := sizeCatalog[strings.ToLower(strings.TrimSpace(key))]; ok {
		return size, true
	}
	return MachineSize{}, false
}
