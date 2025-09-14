package middleware

import (
	"fmt"
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// FeatureGateMiddleware pour bloquer l'accès aux fonctionnalités selon l'abonnement
type FeatureGateMiddleware interface {
	RequireFeature(featureName string) gin.HandlerFunc
	RequireAnyFeature(features ...string) gin.HandlerFunc
}

type featureGateMiddleware struct {
	subscriptionService services.SubscriptionService
}

func NewFeatureGateMiddleware(db *gorm.DB) FeatureGateMiddleware {
	return &featureGateMiddleware{
		subscriptionService: services.NewSubscriptionService(db),
	}
}

// RequireFeature exige qu'une fonctionnalité soit incluse dans l'abonnement
func (fgm *featureGateMiddleware) RequireFeature(featureName string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			ctx.Next()
			return
		}

		subscription, err := fgm.subscriptionService.GetActiveUserSubscription(userId)
		if err != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: fmt.Sprintf("Feature '%s' requires an active subscription", featureName),
			})
			ctx.Abort()
			return
		}

		sPlan, errSPlan := fgm.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
		if errSPlan != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Active subscription required",
			})
			ctx.Abort()
			return
		}

		features := sPlan.Features
		if containsFeature(features, featureName) {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: fmt.Sprintf("Feature '%s' is not included in your current plan", featureName),
			})
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// RequireAnyFeature exige qu'au moins une fonctionnalité soit présente
func (fgm *featureGateMiddleware) RequireAnyFeature(featuresRequired ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			ctx.Next()
			return
		}

		subscription, err := fgm.subscriptionService.GetActiveUserSubscription(userId)
		if err != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "This feature requires an active subscription",
			})
			ctx.Abort()
			return
		}

		sPlan, errSPlan := fgm.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
		if errSPlan != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Active subscription required",
			})
			ctx.Abort()
			return
		}

		features := sPlan.Features
		hasRequiredFeature := false

		for _, requiredFeature := range featuresRequired {
			if containsFeature(features, requiredFeature) {
				hasRequiredFeature = true
				break
			}
		}

		if !hasRequiredFeature {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "This feature is not included in your current plan",
			})
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}
