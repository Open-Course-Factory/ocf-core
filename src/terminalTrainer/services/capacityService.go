// Package services — capacity evaluation logic.
//
// This file extracts the launch-capacity decision out of the
// CheckRAMAvailability middleware so the same logic can be exposed as a
// no-side-effect query endpoint (GET /terminals/capacity-check). Both the
// middleware and the endpoint call EvaluateLaunchCapacity to guarantee the
// frontend sees the exact same answer the backend would enforce on launch.
package services

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/catalog"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/utils"
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

// sizeRAMGB returns the per-instance RAM estimate (in GB) for a size key,
// derived from the canonical catalog (catalog.LookupSize). Used by the
// capacity evaluator to keep capacity decisions aligned with the budget
// engine — a drift here would let users bypass the budget at launch time.
//
// Returns (0, false) for unknown keys; callers handle the miss explicitly.
func sizeRAMGB(key string) (float64, bool) {
	size, ok := catalog.LookupSize(key)
	if !ok {
		return 0, false
	}
	return float64(size.MemoryMB) / 1024.0, true
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
// it is set; otherwise falls back to the catalog's largest entry as a
// conservative worst-case (matches the spirit of the legacy middleware,
// which used the plan's max-allowed size).
func resolveRequiredRAM(_ *paymentModels.SubscriptionPlan, requestedSize string) float64 {
	if requestedSize != "" {
		if ram, ok := sizeRAMGB(requestedSize); ok {
			return ram
		}
	}
	return float64(catalog.LargestSize().MemoryMB) / 1024.0
}

// EstimatePerTerminalRAMGB returns the per-terminal RAM estimate in GB
// used by bulk pre-flight checks. Without a per-request size to anchor
// on, we use the catalog's largest entry as a conservative worst-case.
// Exported so it can be unit-tested directly and reused by other
// pre-flight callers.
func EstimatePerTerminalRAMGB() float64 {
	return float64(catalog.LargestSize().MemoryMB) / 1024.0
}

// MetricsFetcher is the narrow surface EnforceLaunchCapacity needs from the
// terminal service. Defined here to keep capacityService.go a leaf package
// (no upward dependency on the full TerminalTrainerService interface).
type MetricsFetcher interface {
	GetServerMetrics(nocache bool, backend string) (*dto.ServerMetricsResponse, error)
}

// EnforceLaunchCapacity is the SSOT for "would launching/resuming a session
// of `sizeKey` exceed the host's safe RAM headroom?" used by the resume and
// scenario-launch controllers. Writes a 503 + canonical "Server at capacity"
// message to ctx and returns true when the launch must be rejected; returns
// false to let the caller proceed.
//
// Permissive on missing inputs (nil plan, metrics fetch error) — matches the
// historical CheckRAMAvailability behavior: never block on infrastructure
// flakiness, only on confirmed insufficient host RAM.
//
// Callers MUST short-circuit on `true`:
//
//	if services.EnforceLaunchCapacity(ctx, plan, sizeKey, svc) { return }
//
// The read-only /terminals/capacity-check probe calls EvaluateLaunchCapacity
// directly because it returns the structured result to the frontend, not a
// 503 — do not route it through this helper.
func EnforceLaunchCapacity(ctx *gin.Context, plan *paymentModels.SubscriptionPlan, sizeKey string, fetcher MetricsFetcher) bool {
	if plan == nil {
		return false
	}
	metrics, err := fetcher.GetServerMetrics(true, "")
	if err != nil {
		utils.Warn("capacity guard: metrics unavailable, allowing launch: %v", err)
		return false
	}
	result := EvaluateLaunchCapacity(plan, sizeKey, metrics)
	if result.Status != CapacityStatusCritical {
		return false
	}
	msg := "Server at capacity. Please try again later."
	if result.Reason == "ram_full" {
		msg = "Server at capacity: RAM fully utilized. Please try again later."
	} else {
		utils.Warn("capacity guard: launch blocked for size=%q (%.2f GB available)",
			sizeKey, metrics.RAMAvailableGB)
	}
	ctx.JSON(http.StatusServiceUnavailable, &errors.APIError{
		ErrorCode:    http.StatusServiceUnavailable,
		ErrorMessage: msg,
	})
	return true
}
