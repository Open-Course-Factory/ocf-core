// src/payment/middleware/featureMiddleware.go
package middleware

import (
	"net/http"
	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type FeatureMiddleware struct {
	effectivePlanService services.EffectivePlanService
}

func NewFeatureMiddleware(db *gorm.DB) *FeatureMiddleware {
	return &FeatureMiddleware{
		effectivePlanService: services.NewEffectivePlanService(db),
	}
}

// RequireFeature checks if the user has access to a feature via their effective plan
// (unified resolution: org subscription with personal fallback).
func (fm *FeatureMiddleware) RequireFeature(featureName string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userID := ctx.GetString("userId")

		if userID == "" {
			ctx.JSON(http.StatusUnauthorized, &errors.APIError{
				ErrorCode:    http.StatusUnauthorized,
				ErrorMessage: "User not authenticated",
			})
			ctx.Abort()
			return
		}

		result, err := fm.effectivePlanService.GetUserEffectivePlan(userID)
		if err != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "No active subscription found. Please subscribe to a plan that includes " + featureName,
			})
			ctx.Abort()
			return
		}

		// Check if the feature is in the effective plan's features list
		hasFeature := false
		for _, feature := range result.Plan.Features {
			if feature == featureName {
				hasFeature = true
				break
			}
		}

		if !hasFeature {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Your current plan does not include " + featureName + ". Please upgrade your subscription.",
			})
			ctx.Abort()
			return
		}

		// Feature is available, continue
		ctx.Next()
	}
}

// RequireGroupManagement is a convenience wrapper for group management feature
func (fm *FeatureMiddleware) RequireGroupManagement() gin.HandlerFunc {
	return fm.RequireFeature("group_management")
}

// RequireBulkPurchase is a convenience wrapper for bulk purchase feature
func (fm *FeatureMiddleware) RequireBulkPurchase() gin.HandlerFunc {
	return fm.RequireFeature("bulk_purchase")
}
