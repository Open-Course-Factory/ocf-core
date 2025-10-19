// src/payment/middleware/featureMiddleware.go
package middleware

import (
	"net/http"
	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/repositories"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type FeatureMiddleware struct {
	paymentRepo repositories.PaymentRepository
}

func NewFeatureMiddleware(db *gorm.DB) *FeatureMiddleware {
	return &FeatureMiddleware{
		paymentRepo: repositories.NewPaymentRepository(db),
	}
}

// RequireFeature checks if the user's active subscription includes the specified feature
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

		// Get user's active subscription
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
		hasFeature := false
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
