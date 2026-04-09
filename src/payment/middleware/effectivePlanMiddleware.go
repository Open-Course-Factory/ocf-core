package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InjectOrgContext peeks at the request body to extract organization_id and
// stores it in the Gin context as "org_context_id". The body is reset so
// downstream handlers can still read it.
func InjectOrgContext() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// Check query parameter first (for GET requests)
		if orgID := ctx.Query("organization_id"); orgID != "" {
			ctx.Set("org_context_id", orgID)
			ctx.Next()
			return
		}

		bodyBytes, err := io.ReadAll(ctx.Request.Body)
		if err != nil {
			ctx.Next()
			return
		}
		// Reset body for downstream handlers
		ctx.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Parse organization_id from JSON body (for POST requests)
		var partial struct {
			OrganizationID string `json:"organization_id"`
		}
		if json.Unmarshal(bodyBytes, &partial) == nil && partial.OrganizationID != "" {
			ctx.Set("org_context_id", partial.OrganizationID)
		}
		ctx.Next()
	}
}

// InjectEffectivePlan resolves the user's effective subscription plan and stores
// it in the request context. Downstream middleware (RequirePlan, CheckLimit) can
// then read it without repeating the resolution logic.
// The db parameter is used for the admin bypass fallback (see issue #239).
func InjectEffectivePlan(effectivePlanService services.EffectivePlanService, db ...*gorm.DB) gin.HandlerFunc {
	var adminDB *gorm.DB
	if len(db) > 0 {
		adminDB = db[0]
	}
	return func(ctx *gin.Context) {
		userID := ctx.GetString("userId")
		if userID == "" {
			// No authenticated user — let auth middleware handle it
			ctx.Next()
			return
		}

		// Check for org context (set by InjectOrgContext middleware)
		var orgID *uuid.UUID
		if orgContextStr, exists := ctx.Get("org_context_id"); exists {
			if parsed, err := uuid.Parse(orgContextStr.(string)); err == nil {
				orgID = &parsed
			}
		}

		result, err := effectivePlanService.GetUserEffectivePlanForOrg(userID, orgID)
		if err != nil {
			// Admin bypass: if plan resolution failed (e.g. admin is not a member
			// of the org), resolve the org's subscription directly.
			// See issue #239 for the cleaner service-level refactor.
			roles, _ := ctx.Get("userRoles")
			userRoles, _ := roles.([]string)
			if orgID != nil && adminDB != nil && access.IsAdmin(userRoles) {
				result = resolveOrgPlanForAdmin(adminDB, *orgID)
			}

			if result == nil {
				ctx.Set("effective_plan_result", (*services.EffectivePlanResult)(nil))
				utils.Debug("No effective plan for user %s (org context: %v): %v", userID, orgID, err)
				ctx.Next()
				return
			}
		}

		// Store result, source, and backward-compatible plan reference
		ctx.Set("effective_plan_result", result)
		ctx.Set("subscription_plan", result.Plan)
		ctx.Set("planSource", string(result.Source))
		ctx.Next()
	}
}

// resolveOrgPlanForAdmin fetches the org's subscription directly, bypassing
// the membership check that the normal effective plan service enforces.
// Returns nil if the org has no active subscription.
func resolveOrgPlanForAdmin(db *gorm.DB, orgID uuid.UUID) *services.EffectivePlanResult {
	var orgSub models.OrganizationSubscription
	err := db.Preload("SubscriptionPlan").
		Where("organization_id = ? AND status IN ?", orgID, []string{"active", "trialing"}).
		First(&orgSub).Error
	if err != nil {
		utils.Debug("Admin fallback: no active subscription for org %s: %v", orgID, err)
		return nil
	}
	return &services.EffectivePlanResult{
		Plan:                     &orgSub.SubscriptionPlan,
		Source:                   services.PlanSourceOrganization,
		OrganizationSubscription: &orgSub,
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

		// Check for org context (set by InjectOrgContext middleware)
		var orgID *uuid.UUID
		if orgContextStr, exists := ctx.Get("org_context_id"); exists {
			if parsed, err := uuid.Parse(orgContextStr.(string)); err == nil {
				orgID = &parsed
			}
		}

		limitCheck, err := effectivePlanService.CheckEffectiveUsageLimitForOrg(userID, orgID, metricType, 1)
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
