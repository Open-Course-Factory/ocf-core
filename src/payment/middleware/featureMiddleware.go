// src/payment/middleware/featureMiddleware.go
package middleware

import (
	"net/http"
	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type FeatureMiddleware struct {
	paymentRepo   repositories.PaymentRepository
	orgSubService services.OrganizationSubscriptionService
	db            *gorm.DB
}

func NewFeatureMiddleware(db *gorm.DB) *FeatureMiddleware {
	return &FeatureMiddleware{
		paymentRepo:   repositories.NewPaymentRepository(db),
		orgSubService: services.NewOrganizationSubscriptionService(db),
		db:            db,
	}
}

// RequireFeature checks if the user has access to a feature through organization or user subscription
// Phase 2: Checks organization subscriptions first, then falls back to user subscriptions (backward compat)
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

		// Phase 2: Check organization subscriptions first
		hasFeature, err := fm.orgSubService.CanUserAccessFeature(userID, featureName)
		if err == nil && hasFeature {
			// User has access via organization subscription
			ctx.Next()
			return
		}

		// Backward compatibility: Fall back to user subscription (deprecated)
		subscription, err := fm.paymentRepo.GetActiveUserSubscription(userID)
		if err != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "No active subscription found. Please subscribe to a plan that includes " + featureName,
			})
			ctx.Abort()
			return
		}

		// Check if the feature is in the plan's features list
		hasFeature = false
		for _, feature := range subscription.SubscriptionPlan.Features {
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
