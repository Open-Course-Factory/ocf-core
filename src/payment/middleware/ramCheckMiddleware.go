package middleware

import (
	"fmt"
	"net/http"

	"soli/formations/src/auth/errors"
	paymentModels "soli/formations/src/payment/models"
	terminalServices "soli/formations/src/terminalTrainer/services"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
)

// CheckRAMAvailability verifies that the Terminal Trainer server has enough RAM
// to create a new terminal. It reads the subscription_plan from the context
// (set by InjectEffectivePlan) to estimate RAM requirements based on allowed
// machine sizes.
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

		// Get real-time server metrics (nocache=true)
		metrics, err := terminalService.GetServerMetrics(true, "")
		if err != nil {
			// Log the error but don't block terminal creation if the metrics service is unavailable
			utils.Warn("Failed to check server metrics for terminal creation: %v", err)
			ctx.Next()
			return
		}

		// Map machine sizes to required RAM (GB)
		machineSizeToRAM := map[string]float64{
			"XS": 0.25,
			"S":  0.5,
			"M":  1.0,
			"L":  2.0,
			"XL": 4.0,
		}

		// Estimate RAM based on the largest allowed machine size in the plan
		var estimatedRAM float64 = 0.5 // default for S

		if len(plan.AllowedMachineSizes) > 0 {
			maxRAM := 0.0
			for _, size := range plan.AllowedMachineSizes {
				if size == "all" {
					estimatedRAM = 1.0 // Use M as average for "all"
					break
				}
				if ram, found := machineSizeToRAM[size]; found && ram > maxRAM {
					maxRAM = ram
				}
			}
			if maxRAM > 0 {
				estimatedRAM = maxRAM
			}
		}

		const minRAMReservePercent = 5.0

		// Calculate approximate total RAM from available RAM and usage percentage
		// ram_available_gb = total_ram * (1 - ram_percent/100)
		// therefore: total_ram = ram_available_gb / (1 - ram_percent/100)
		totalRAM := metrics.RAMAvailableGB / (1.0 - metrics.RAMPercent/100.0)
		minReservedRAM := totalRAM * (minRAMReservePercent / 100.0)

		// Check that enough RAM remains after creating the terminal
		ramAfterCreation := metrics.RAMAvailableGB - estimatedRAM

		if ramAfterCreation < minReservedRAM {
			ctx.JSON(http.StatusServiceUnavailable, &errors.APIError{
				ErrorCode: http.StatusServiceUnavailable,
				ErrorMessage: fmt.Sprintf("Server at capacity: insufficient RAM available (%.2f GB available, %.2f GB required for terminal + %.2f GB reserve). Please try again later.",
					metrics.RAMAvailableGB, estimatedRAM, minReservedRAM),
			})
			ctx.Abort()
			return
		}

		// Store metrics in context for downstream use
		ctx.Set("server_metrics", metrics)
		ctx.Next()
	}
}
