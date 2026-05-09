// Tests for terminalServices.EvaluateLaunchCapacity — the pure-function
// kernel of both the CheckRAMAvailability middleware and the new
// /terminals/capacity-check endpoint.
//
// The "uses chosen size not plan max" rows are load-bearing: they cover
// the behaviour change that unblocks high-tier users from launching small
// sizes when RAM is tight.
package terminalTrainer_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"
)

// metricsFor builds a ServerMetricsResponse with the requested availability.
//
//	availableGB — RAM remaining (GB).
//	usedPercent — percent of total RAM currently used (0–100).
//
// The total RAM is derived implicitly: total = availableGB / (1 - used/100).
func metricsFor(availableGB, usedPercent float64) *dto.ServerMetricsResponse {
	return &dto.ServerMetricsResponse{
		RAMAvailableGB: availableGB,
		RAMPercent:     usedPercent,
	}
}

func planAllowing(sizes ...string) *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{AllowedMachineSizes: sizes}
}

func TestEvaluateLaunchCapacity(t *testing.T) {
	tests := []struct {
		name           string
		plan           *paymentModels.SubscriptionPlan
		size           string
		metrics        *dto.ServerMetricsResponse
		expectedStatus services.CapacityStatus
		expectedReason string // empty = don't assert reason
	}{
		// --- Plenty of RAM ---------------------------------------------------
		{
			name:           "plenty of RAM, small size",
			plan:           planAllowing("XS", "S", "M", "L"),
			size:           "XS",
			metrics:        metricsFor(10.0, 50.0), // total = 20GB, reserve = 1GB
			expectedStatus: services.CapacityStatusOK,
		},

		// --- Load-bearing: tight RAM, small size accepted -------------------
		// Pre-refactor: middleware used plan max (L = 2GB) and rejected.
		// Post-refactor: middleware uses chosen size (XS = 0.25GB) → OK.
		// total = 10GB, reserve = 0.5GB, after = 1.7 - 0.25 = 1.45GB > 0.5
		{
			name:           "tight RAM, small size — uses chosen size not plan max",
			plan:           planAllowing("XS", "S", "M", "L"),
			size:           "XS",
			metrics:        metricsFor(1.7, 83.0),
			expectedStatus: services.CapacityStatusOK,
		},

		// --- Tight RAM, requested matches plan max → critical ---------------
		// total ≈ 10GB, reserve = 0.5GB, after = 1.7 - 2.0 = -0.3GB < 0.5
		{
			name:           "tight RAM, requested matches plan max",
			plan:           planAllowing("XS", "S", "M", "L"),
			size:           "L",
			metrics:        metricsFor(1.7, 83.0),
			expectedStatus: services.CapacityStatusCritical,
			expectedReason: "insufficient_ram_for_size",
		},

		// --- Tight RAM, no size requested → falls back to plan max → critical
		{
			name:           "tight RAM, no size requested falls back to plan max",
			plan:           planAllowing("XS", "S", "M", "L"),
			size:           "",
			metrics:        metricsFor(1.7, 83.0),
			expectedStatus: services.CapacityStatusCritical,
			expectedReason: "insufficient_ram_for_size",
		},

		// --- 99% RAM short-circuit -----------------------------------------
		{
			name:           "99% RAM short-circuit",
			plan:           planAllowing("XS"),
			size:           "XS",
			metrics:        metricsFor(0.05, 99.0),
			expectedStatus: services.CapacityStatusCritical,
			expectedReason: "ram_full",
		},
		{
			name:           "100% RAM short-circuit",
			plan:           planAllowing("XS"),
			size:           "XS",
			metrics:        metricsFor(0.0, 100.0),
			expectedStatus: services.CapacityStatusCritical,
			expectedReason: "ram_full",
		},

		// --- Borderline (warning band) -------------------------------------
		// total = 10GB, reserve = 0.5GB; after request must be in
		// [reserve, 1.5*reserve) = [0.5, 0.75). Pick avail=1.6GB, size=M
		// (1.0GB) → after=0.6GB → warning.
		{
			name:           "borderline launch lands in warning band",
			plan:           planAllowing("XS", "S", "M"),
			size:           "M",
			metrics:        metricsFor(1.6, 84.0),
			expectedStatus: services.CapacityStatusWarning,
			expectedReason: "ram_tight",
		},

		// --- Plan nil — uses requested size if known -----------------------
		{
			name:           "nil plan, valid size",
			plan:           nil,
			size:           "XS",
			metrics:        metricsFor(10.0, 50.0),
			expectedStatus: services.CapacityStatusOK,
		},

		// --- Size not in plan — falls back to plan max ----------------------
		// Plan only allows XS (0.25GB). User asks for L (2GB). Falls back
		// to XS estimate (0.25GB) which fits comfortably.
		{
			name:           "size not allowed in plan falls back to plan max",
			plan:           planAllowing("XS"),
			size:           "L",
			metrics:        metricsFor(10.0, 50.0),
			expectedStatus: services.CapacityStatusOK,
		},

		// --- Nil metrics -> unknown ----------------------------------------
		{
			name:           "nil metrics yields unknown",
			plan:           planAllowing("XS"),
			size:           "XS",
			metrics:        nil,
			expectedStatus: services.CapacityStatusUnknown,
			expectedReason: "no_metrics",
		},

		// --- Plan with "all" sentinel -------------------------------------
		// "all" is unbounded; the function reuses the historic 1GB
		// estimate. With 50% RAM available, an XS request via
		// planAllowsSize → returns XS estimate (0.25GB). OK.
		{
			name:           "plan with 'all' sentinel + small size",
			plan:           planAllowing("all"),
			size:           "XS",
			metrics:        metricsFor(10.0, 50.0),
			expectedStatus: services.CapacityStatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := services.EvaluateLaunchCapacity(tc.plan, tc.size, tc.metrics)
			assert.Equal(t, tc.expectedStatus, result.Status, "status mismatch (reason=%q)", result.Reason)
			if tc.expectedReason != "" {
				assert.Equal(t, tc.expectedReason, result.Reason, "reason mismatch")
			}
		})
	}
}
