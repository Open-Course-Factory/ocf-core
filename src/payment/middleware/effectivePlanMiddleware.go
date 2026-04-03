package middleware

import (
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// InjectEffectivePlan resolves the user's effective subscription plan and stores
// it in the request context. Downstream middleware (RequirePlan, CheckLimit) can
// then read it without repeating the resolution logic.
func InjectEffectivePlan(effectivePlanService services.EffectivePlanService) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userID := ctx.GetString("userId")
		if userID == "" {
			// No authenticated user — let auth middleware handle it
			ctx.Next()
			return
		}

		result, err := effectivePlanService.GetUserEffectivePlan(userID)
		if err != nil {
			// No plan found — store nil so RequirePlan can decide what to do
			ctx.Set("effective_plan_result", (*services.EffectivePlanResult)(nil))
			utils.Debug("No effective plan for user %s: %v", userID, err)
			ctx.Next()
			return
		}

		// Store result, source, and backward-compatible plan reference
		ctx.Set("effective_plan_result", result)
		ctx.Set("subscription_plan", result.Plan)
		ctx.Set("planSource", string(result.Source))
		ctx.Next()
	}
}

// RequirePlan aborts the request with 403 if no effective plan was resolved
// by InjectEffectivePlan.
func RequirePlan() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		val, exists := ctx.Get("effective_plan_result")
		if !exists || val == nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Active subscription required",
			})
			ctx.Abort()
			return
		}

		result, ok := val.(*services.EffectivePlanResult)
		if !ok || result == nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Active subscription required",
			})
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// CheckLimit verifies that the user has not exceeded the given metricType limit
// and increments usage after a successful response.
func CheckLimit(effectivePlanService services.EffectivePlanService, db *gorm.DB, metricType string) gin.HandlerFunc {
	paymentRepo := repositories.NewPaymentRepository(db)

	return func(ctx *gin.Context) {
		userID := ctx.GetString("userId")
		if userID == "" {
			ctx.Next()
			return
		}

		limitCheck, err := effectivePlanService.CheckEffectiveUsageLimit(userID, metricType, 1)
		if err != nil {
			utils.Warn("Failed to check usage limit for user %s: %v", userID, err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to check usage limit",
			})
			ctx.Abort()
			return
		}

		if !limitCheck.Allowed {
			ctx.JSON(http.StatusForbidden, gin.H{
				"error_code":    http.StatusForbidden,
				"error_message": limitCheck.Message,
				"source":        string(limitCheck.Source),
			})
			ctx.Abort()
			return
		}

		// Process the request
		ctx.Next()

		// After response: if successful, increment usage
		if ctx.Writer.Status() >= 200 && ctx.Writer.Status() < 300 {
			if incrementErr := paymentRepo.IncrementUsageMetric(userID, metricType, 1); incrementErr != nil {
				utils.Warn("Failed to increment usage metric %s for user %s: %v", metricType, userID, incrementErr)
			}
		}
	}
}
