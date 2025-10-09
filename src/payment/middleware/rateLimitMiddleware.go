package middleware

import (
	"net/http"
	"strings"

	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RateLimitMiddleware basé sur l'abonnement
type SubscriptionBasedRateLimitMiddleware interface {
	ApplyRateLimit() gin.HandlerFunc
}

type subscriptionBasedRateLimitMiddleware struct {
	subscriptionService services.UserSubscriptionService
}

func NewSubscriptionBasedRateLimitMiddleware(db *gorm.DB) SubscriptionBasedRateLimitMiddleware {
	return &subscriptionBasedRateLimitMiddleware{
		subscriptionService: services.NewSubscriptionService(db),
	}
}

// ApplyRateLimit applique un rate limit basé sur le plan d'abonnement
func (srl *subscriptionBasedRateLimitMiddleware) ApplyRateLimit() gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			ctx.Next()
			return
		}

		// Récupérer le plan d'abonnement
		subscription, err := srl.subscriptionService.GetActiveUserSubscription(userId)

		sPlan, errSPlan := srl.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
		if errSPlan != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Active subscription required",
			})
			ctx.Abort()
			return
		}

		var requestsPerMinute int
		if err != nil {
			// Utilisateur sans abonnement = limite très restrictive
			requestsPerMinute = 10
		} else {
			// Déterminer la limite selon le plan
			switch {
			case strings.Contains(sPlan.RequiredRole, "enterprise"):
				requestsPerMinute = 1000
			case strings.Contains(sPlan.RequiredRole, "organization"):
				requestsPerMinute = 500
			case strings.Contains(sPlan.RequiredRole, "premium") ||
				strings.Contains(sPlan.RequiredRole, "pro"):
				requestsPerMinute = 200
			default:
				requestsPerMinute = 60 // Plan de base
			}
		}

		// Implémenter la logique de rate limiting
		// (Vous pouvez utiliser Redis ou un système en mémoire)
		ctx.Set("rate_limit", requestsPerMinute)

		// Pour l'instant, on laisse passer toutes les requêtes
		// Dans une implémentation réelle, vous voudrez :
		// 1. Vérifier le compteur de requêtes pour cet utilisateur
		// 2. Si dépassé, retourner HTTP 429 Too Many Requests
		// 3. Sinon, incrémenter le compteur et continuer

		ctx.Next()
	})
}
