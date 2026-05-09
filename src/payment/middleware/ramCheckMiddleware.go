package middleware

import (
	"net/http"

	"soli/formations/src/auth/errors"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	terminalServices "soli/formations/src/terminalTrainer/services"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// CheckRAMAvailability verifies that the Terminal Trainer server has enough RAM
// to create a new terminal. It reads the subscription_plan from the context
// (set by InjectEffectivePlan) AND the chosen size from the request body so the
// estimate matches the actual allocation rather than always using the plan max
// (which would over-reject high-tier users launching small sizes).
//
// The capacity decision itself lives in
// terminalServices.EvaluateLaunchCapacity so the same logic backs the
// GET /terminals/capacity-check query endpoint.
func CheckRAMAvailability(terminalService terminalServices.TerminalTrainerService) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// Read subscription_plan from context (set by InjectEffectivePlan)
		planVal, exists := ctx.Get("subscription_plan")
		if !exists || planVal == nil {
			// No plan in context — RequirePlan should have blocked earlier
			ctx.Next()
			return
		}

		plan, ok := planVal.(*paymentModels.SubscriptionPlan)
		if !ok || plan == nil {
			ctx.Next()
			return
		}

		// Read the chosen size from the request body (when present) so we
		// estimate RAM for the actual allocation, not the plan's max.
		// ShouldBindBodyWith re-buffers the body so the handler can re-read
		// it. Body parse errors are non-fatal here — let the handler return
		// the canonical 400; we just fall back to the plan-max estimate.
		chosenSize := ""
		var input dto.CreateComposedSessionInput
		if err := ctx.ShouldBindBodyWith(&input, binding.JSON); err == nil {
			chosenSize = input.Size
		}

		// Get real-time server metrics (nocache=true)
		metrics, err := terminalService.GetServerMetrics(true, "")
		if err != nil {
			// Log the error but don't block terminal creation if the metrics service is unavailable
			utils.Warn("Failed to check server metrics for terminal creation: %v", err)
			ctx.Next()
			return
		}

		result := terminalServices.EvaluateLaunchCapacity(plan, chosenSize, metrics)

		if result.Status == terminalServices.CapacityStatusCritical {
			// Preserve the previous user-facing messages so frontend error
			// handling stays bit-compatible.
			msg := "Server at capacity. Please try again later."
			if result.Reason == "ram_full" {
				msg = "Server at capacity: RAM fully utilized. Please try again later."
			} else {
				utils.Warn("Terminal creation blocked: insufficient RAM for size=%q (%.2f GB available)",
					chosenSize, metrics.RAMAvailableGB)
			}
			ctx.JSON(http.StatusServiceUnavailable, &errors.APIError{
				ErrorCode:    http.StatusServiceUnavailable,
				ErrorMessage: msg,
			})
			ctx.Abort()
			return
		}

		// Store metrics in context for downstream use
		ctx.Set("server_metrics", metrics)
		ctx.Next()
	}
}
