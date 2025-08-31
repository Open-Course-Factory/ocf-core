// src/payment/middleware/usageLimitMiddleware.go
package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UsageLimitMiddleware interface {
	CheckCourseCreationLimit() gin.HandlerFunc
	CheckLabSessionLimit() gin.HandlerFunc
	CheckConcurrentUserLimit() gin.HandlerFunc
	CheckCustomLimit(metricType string, increment int64) gin.HandlerFunc
	CheckUsageForPath() gin.HandlerFunc // Middleware automatique basé sur le path
}

type usageLimitMiddleware struct {
	subscriptionService services.SubscriptionService
}

func NewUsageLimitMiddleware(db *gorm.DB) UsageLimitMiddleware {
	return &usageLimitMiddleware{
		subscriptionService: services.NewSubscriptionService(db),
	}
}

// CheckCourseCreationLimit vérifie la limite de création de cours
func (ulm *usageLimitMiddleware) CheckCourseCreationLimit() gin.HandlerFunc {
	return ulm.CheckCustomLimit("courses_created", 1)
}

// CheckLabSessionLimit vérifie la limite de sessions de lab
func (ulm *usageLimitMiddleware) CheckLabSessionLimit() gin.HandlerFunc {
	return ulm.CheckCustomLimit("lab_sessions", 1)
}

// CheckConcurrentUserLimit vérifie la limite d'utilisateurs concurrents
func (ulm *usageLimitMiddleware) CheckConcurrentUserLimit() gin.HandlerFunc {
	return ulm.CheckCustomLimit("concurrent_users", 1)
}

// CheckCustomLimit middleware générique pour vérifier une limite spécifique
func (ulm *usageLimitMiddleware) CheckCustomLimit(metricType string, increment int64) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			// Pas d'authentification, laisser passer (sera géré par le middleware d'auth)
			ctx.Next()
			return
		}

		// Vérifier la limite
		limitCheck, err := ulm.subscriptionService.CheckUsageLimit(userId, metricType, increment)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: fmt.Sprintf("Failed to check usage limit: %v", err),
			})
			ctx.Abort()
			return
		}

		if !limitCheck.Allowed {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: limitCheck.Message,
			})
			ctx.Abort()
			return
		}

		// Stocker les infos pour l'incrémenter après succès
		ctx.Set("usage_metric_type", metricType)
		ctx.Set("usage_increment", increment)
		ctx.Set("usage_check_passed", true)

		ctx.Next()

		// Si la requête a réussi (status 2xx), incrémenter la métrique
		if ctx.Writer.Status() >= 200 && ctx.Writer.Status() < 300 {
			if passed, exists := ctx.Get("usage_check_passed"); exists && passed.(bool) {
				err := ulm.subscriptionService.IncrementUsage(userId, metricType, increment)
				if err != nil {
					// Log l'erreur mais ne pas faire échouer la requête
					fmt.Printf("Warning: Failed to increment usage metric %s for user %s: %v\n", metricType, userId, err)
				}
			}
		}
	}
}

// CheckUsageForPath middleware automatique qui détermine le type de métrique selon le path
func (ulm *usageLimitMiddleware) CheckUsageForPath() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" || ctx.Request.Method != "POST" {
			ctx.Next()
			return
		}

		path := ctx.FullPath()
		var metricType string
		var increment int64 = 1

		// Déterminer le type de métrique selon le path
		switch {
		case strings.Contains(path, "/courses") && strings.HasSuffix(path, "/"):
			metricType = "courses_created"
		case strings.Contains(path, "/terminals/start-session"):
			metricType = "lab_sessions"
		case strings.Contains(path, "/sessions") && strings.HasSuffix(path, "/"):
			metricType = "lab_sessions"
		default:
			// Pas de limite pour ce path
			ctx.Next()
			return
		}

		// Appliquer le middleware de vérification
		limitMiddleware := ulm.CheckCustomLimit(metricType, increment)
		limitMiddleware(ctx)
	}
}

// UserRoleMiddleware pour vérifier les rôles basés sur les abonnements
type UserRoleMiddleware interface {
	EnsureSubscriptionRole() gin.HandlerFunc
	RequirePremiumSubscription() gin.HandlerFunc
	RequireActiveSubscription() gin.HandlerFunc
}

type userRoleMiddleware struct {
	subscriptionService services.SubscriptionService
}

func NewUserRoleMiddleware(db *gorm.DB) UserRoleMiddleware {
	return &userRoleMiddleware{
		subscriptionService: services.NewSubscriptionService(db),
	}
}

// EnsureSubscriptionRole met à jour le rôle de l'utilisateur selon son abonnement
func (urm *userRoleMiddleware) EnsureSubscriptionRole() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId != "" {
			// Mettre à jour le rôle de l'utilisateur selon son abonnement
			err := urm.subscriptionService.UpdateUserRoleBasedOnSubscription(userId)
			if err != nil {
				fmt.Printf("Warning: Failed to update user role: %v\n", err)
			}
		}

		ctx.Next()
	}
}

// RequirePremiumSubscription exige un abonnement premium
func (urm *userRoleMiddleware) RequirePremiumSubscription() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			ctx.JSON(http.StatusUnauthorized, &errors.APIError{
				ErrorCode:    http.StatusUnauthorized,
				ErrorMessage: "Authentication required",
			})
			ctx.Abort()
			return
		}

		subscription, err := urm.subscriptionService.GetActiveUserSubscription(userId)
		if err != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Premium subscription required",
			})
			ctx.Abort()
			return
		}

		// Vérifier que c'est un plan premium (exemple de logique)
		isPremium := strings.Contains(strings.ToLower(subscription.SubscriptionPlan.RequiredRole), "premium") ||
			subscription.SubscriptionPlan.PriceAmount > 1000 // Plus de 10€/mois = premium

		if !isPremium {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Premium subscription required for this feature",
			})
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// RequireActiveSubscription exige un abonnement actif (payant)
func (urm *userRoleMiddleware) RequireActiveSubscription() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			ctx.Next() // Sera géré par le middleware d'auth
			return
		}

		hasActive, err := urm.subscriptionService.HasActiveSubscription(userId)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to check subscription status",
			})
			ctx.Abort()
			return
		}

		if !hasActive {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Active subscription required for this feature",
			})
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// Helper middleware pour incrémenter automatiquement l'usage après succès
type UsageTracker interface {
	TrackUsageAfterSuccess(metricType string, increment int64) gin.HandlerFunc
}

type usageTracker struct {
	subscriptionService services.SubscriptionService
}

func NewUsageTracker(db *gorm.DB) UsageTracker {
	return &usageTracker{
		subscriptionService: services.NewSubscriptionService(db),
	}
}

func (ut *usageTracker) TrackUsageAfterSuccess(metricType string, increment int64) gin.HandlerFunc {
	return gin.HandlerFunc(func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		// Traiter la requête d'abord
		ctx.Next()

		// Si succès, incrémenter la métrique
		if ctx.Writer.Status() >= 200 && ctx.Writer.Status() < 300 && userId != "" {
			err := ut.subscriptionService.IncrementUsage(userId, metricType, increment)
			if err != nil {
				fmt.Printf("Warning: Failed to track usage %s for user %s: %v\n", metricType, userId, err)
			}
		}
	})
}
