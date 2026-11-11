// Package services — capacity evaluation logic.
//
// EvaluateLaunchCapacity is the one host RAM rule: the launch paths enforce it
// and GET /terminals/capacity-check reports it, so the frontend sees the
// answer the backend would enforce on launch.
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
	// CapacityStatusCritical means the launch would be refused for lack of
	// host RAM.
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

// minRAMReserveFraction is the share of a host's RAM no launch may take.
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
//   - Returns CapacityStatusCritical when launch would be rejected (503).
//   - Returns CapacityStatusWarning when launch is borderline (within
//     ~1.5x of the reject threshold) — surfaces an "approaching capacity"
//     hint to the user.
//   - Returns CapacityStatusOK when comfortably within capacity.
//   - Returns CapacityStatusUnknown when metrics are unavailable.
//
// requestedSize is the user's chosen machine size (XS/S/M/L/XL). When
// empty, falls back to the catalog's largest size, so a missing size never
// relaxes the check.
func EvaluateLaunchCapacity(plan *paymentModels.SubscriptionPlan, requestedSize string, metrics *dto.ServerMetricsResponse) CapacityResult {
	if metrics == nil {
		return CapacityResult{Status: CapacityStatusUnknown, Reason: "no_metrics"}
	}

	if sessionsThatFit(plan, requestedSize, metrics) == 0 {
		if ramFull(metrics) {
			return CapacityResult{Status: CapacityStatusCritical, Reason: "ram_full"}
		}
		return CapacityResult{Status: CapacityStatusCritical, Reason: "insufficient_ram_for_size"}
	}

	minReservedRAM := metrics.TotalRAMGB() * minRAMReserveFraction
	ramAfterCreation := metrics.RAMAvailableGB - resolveRequiredRAM(plan, requestedSize)
	// Borderline band: within 1.5x of the reserve buffer. Surfaces a
	// "tight capacity" hint to the user without blocking the launch.
	if ramAfterCreation < minReservedRAM*1.5 {
		return CapacityResult{Status: CapacityStatusWarning, Reason: "ram_tight"}
	}
	return CapacityResult{Status: CapacityStatusOK, Reason: "ok"}
}

// sessionsThatFit counts the sessions of size a host can still take, one
// launch after another, keeping the reserve free. It is the rule the launch
// gate refuses on: none fits exactly when EvaluateLaunchCapacity is critical.
func sessionsThatFit(plan *paymentModels.SubscriptionPlan, size string, metrics *dto.ServerMetricsResponse) int {
	// Even an XS session is refused once the server is at >=99% RAM, and with
	// no reported total there is no headroom to reason about.
	if ramFull(metrics) {
		return 0
	}
	spare := metrics.RAMAvailableGB - metrics.TotalRAMGB()*minRAMReserveFraction
	if spare <= 0 {
		return 0
	}
	return int(spare / resolveRequiredRAM(plan, size))
}

func ramFull(metrics *dto.ServerMetricsResponse) bool {
	return metrics.RAMPercent >= 99.0 || metrics.TotalRAMGB() <= 0
}

// resolveRequiredRAM picks the RAM estimate for the requested size when
// it is set; otherwise falls back to the catalog's largest entry as a
// conservative worst-case.
func resolveRequiredRAM(_ *paymentModels.SubscriptionPlan, requestedSize string) float64 {
	if requestedSize != "" {
		if ram, ok := sizeRAMGB(requestedSize); ok {
			return ram
		}
	}
	return float64(catalog.LargestSize().MemoryMB) / 1024.0
}

// MetricsFetcher is the narrow surface EnforceLaunchCapacity needs from the
// terminal service. Defined here to keep capacityService.go a leaf package
// (no upward dependency on the full TerminalTrainerService interface).
type MetricsFetcher interface {
	GetServerMetrics(nocache bool, backend string) (*dto.ServerMetricsResponse, error)
}

// EnforceLaunchCapacity answers "would resuming a session of `sizeKey` on
// `backend` exceed that host's safe RAM headroom?" for the resume paths. A new
// session's host is chosen, and checked, by StartComposedSession instead.
// Writes a 503 + canonical "Server at capacity" message to ctx and returns true
// when the resume must be rejected; returns false to let the caller proceed.
//
// Permissive on missing inputs (nil plan, metrics fetch error) — matches the
// historical CheckRAMAvailability behavior: never block on infrastructure
// flakiness, only on confirmed insufficient host RAM.
//
// Callers MUST short-circuit on `true`:
//
//	if services.EnforceLaunchCapacity(ctx, plan, sizeKey, terminal.Backend, svc) { return }
//
// The read-only /terminals/capacity-check probe calls EvaluateLaunchCapacity
// directly because it returns the structured result to the frontend, not a
// 503 — do not route it through this helper.
func EnforceLaunchCapacity(ctx *gin.Context, plan *paymentModels.SubscriptionPlan, sizeKey string, backend string, fetcher MetricsFetcher) bool {
	if plan == nil {
		return false
	}
	metrics, err := fetcher.GetServerMetrics(true, backend)
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
	errors.Respond(ctx, http.StatusServiceUnavailable, msg)
	return true
}
