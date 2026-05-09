// Package services — capacity evaluation logic.
//
// This file extracts the launch-capacity decision out of the
// CheckRAMAvailability middleware so the same logic can be exposed as a
// no-side-effect query endpoint (GET /terminals/capacity-check). Both the
// middleware and the endpoint call EvaluateLaunchCapacity to guarantee the
// frontend sees the exact same answer the backend would enforce on launch.
package services

import (
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
)

// CapacityStatus is a coarse-grained launch readiness signal.
type CapacityStatus string

const (
	// CapacityStatusOK means a launch should succeed.
	CapacityStatusOK CapacityStatus = "ok"
	// CapacityStatusWarning means a launch should still succeed but the
	// server is approaching its safety reserve — surfaces a hint to the
	// user that capacity is tight.
	CapacityStatusWarning CapacityStatus = "warning"
	// CapacityStatusCritical means the launch would be rejected by the
	// CheckRAMAvailability middleware.
	CapacityStatusCritical CapacityStatus = "critical"
	// CapacityStatusUnknown means metrics could not be evaluated (no
	// data, transient backend failure, etc.). The frontend should treat
	// this as "allow but show a neutral indicator".
	CapacityStatusUnknown CapacityStatus = "unknown"
)

// CapacityResult is the canonical answer returned by EvaluateLaunchCapacity
// and surfaced verbatim by GET /terminals/capacity-check.
type CapacityResult struct {
	Status CapacityStatus `json:"status"`
	Reason string         `json:"reason"`
}

// minRAMReserveFraction matches the 5% reserve previously hard-coded in
// CheckRAMAvailability — kept identical so the middleware behaviour is
// preserved bit-for-bit when no size is passed in the request body.
const minRAMReserveFraction = 0.05

// machineSizeToRAM is the canonical mapping of machine size keys to
// expected RAM consumption in GB. Mirrors the table already used by
// CheckRAMAvailability before the refactor.
var machineSizeToRAM = map[string]float64{
	"XS": 0.25,
	"S":  0.5,
	"M":  1.0,
	"L":  2.0,
	"XL": 4.0,
}

// EvaluateLaunchCapacity computes whether a session of the given size could
// be launched right now, given a plan and current server metrics.
//
//   - Returns CapacityStatusCritical when launch would be rejected
//     (matches CheckRAMAvailability's 503 path).
//   - Returns CapacityStatusWarning when launch is borderline (within
//     ~1.5x of the reject threshold) — surfaces an "approaching capacity"
//     hint to the user.
//   - Returns CapacityStatusOK when comfortably within capacity.
//   - Returns CapacityStatusUnknown when metrics are unavailable.
//
// requestedSize is the user's chosen machine size (XS/S/M/L/XL). When
// empty, falls back to the plan's max allowed size — preserves the
// pre-refactor middleware behaviour for callers that don't pass a size
// (defensive: an unparseable request body shouldn't accidentally relax
// the check).
func EvaluateLaunchCapacity(plan *paymentModels.SubscriptionPlan, requestedSize string, metrics *dto.ServerMetricsResponse) CapacityResult {
	if metrics == nil {
		return CapacityResult{Status: CapacityStatusUnknown, Reason: "no_metrics"}
	}

	// Preserve the existing "RAM saturated" short-circuit: even an XS
	// session is refused once the server is at >=99% RAM.
	if metrics.RAMPercent >= 99.0 {
		return CapacityResult{Status: CapacityStatusCritical, Reason: "ram_full"}
	}

	requiredRAM := resolveRequiredRAM(plan, requestedSize)

	// Recover the total RAM (GB) from the available + percentage pair.
	// ram_available_gb = total_ram * (1 - ram_percent/100)
	// Guard against division-by-zero (RAMPercent < 100 is enforced above
	// but we use 99 as the cutoff and keep a defensive denominator).
	denom := 1.0 - metrics.RAMPercent/100.0
	if denom <= 0 {
		return CapacityResult{Status: CapacityStatusCritical, Reason: "ram_full"}
	}
	totalRAM := metrics.RAMAvailableGB / denom
	minReservedRAM := totalRAM * minRAMReserveFraction

	ramAfterCreation := metrics.RAMAvailableGB - requiredRAM

	if ramAfterCreation < minReservedRAM {
		return CapacityResult{Status: CapacityStatusCritical, Reason: "insufficient_ram_for_size"}
	}
	// Borderline band: within 1.5x of the reserve buffer. Surfaces a
	// "tight capacity" hint to the user without blocking the launch.
	if ramAfterCreation < minReservedRAM*1.5 {
		return CapacityResult{Status: CapacityStatusWarning, Reason: "ram_tight"}
	}
	return CapacityResult{Status: CapacityStatusOK, Reason: "ok"}
}

// resolveRequiredRAM picks the RAM estimate for the requested size when
// it is set AND the plan allows it; otherwise falls back to the plan's
// max allowed size (keeps the legacy middleware behaviour). Defaults to
// the "S" estimate (0.5 GB) when no plan information is available — same
// default as the pre-refactor middleware.
func resolveRequiredRAM(plan *paymentModels.SubscriptionPlan, requestedSize string) float64 {
	if requestedSize != "" {
		if ram, ok := machineSizeToRAM[requestedSize]; ok && planAllowsSize(plan, requestedSize) {
			return ram
		}
	}

	if plan != nil && len(plan.AllowedMachineSizes) > 0 {
		maxRAM := 0.0
		for _, size := range plan.AllowedMachineSizes {
			if size == "all" {
				// "all" is unbounded by definition — fall back to the
				// historic average of M (1 GB) used by the legacy
				// middleware.
				return 1.0
			}
			if ram, ok := machineSizeToRAM[size]; ok && ram > maxRAM {
				maxRAM = ram
			}
		}
		if maxRAM > 0 {
			return maxRAM
		}
	}
	return 0.5
}

// planAllowsSize reports whether the plan permits the given machine size.
// A nil plan or empty AllowedMachineSizes list is treated as unrestricted
// (matches GetSessionOptions semantics elsewhere in the codebase).
func planAllowsSize(plan *paymentModels.SubscriptionPlan, size string) bool {
	if plan == nil {
		return true
	}
	if len(plan.AllowedMachineSizes) == 0 {
		return true
	}
	for _, s := range plan.AllowedMachineSizes {
		if s == "all" || s == size {
			return true
		}
	}
	return false
}
