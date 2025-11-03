// src/payment/middleware/subscriptionIntegrationMiddleware.go
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

type SubscriptionIntegrationMiddleware interface {
	// Middlewares pour actions spécifiques
	RequireSubscriptionForCourseCreation() gin.HandlerFunc
	RequireSubscriptionForAdvancedLabs() gin.HandlerFunc
	RequireSubscriptionForAPI() gin.HandlerFunc

	// Middleware générique pour vérifier l'abonnement selon le contexte
	CheckSubscriptionRequirements() gin.HandlerFunc

	// Middleware pour injecter les informations d'abonnement dans le contexte
	InjectSubscriptionInfo() gin.HandlerFunc
}

type subscriptionIntegrationMiddleware struct {
	subscriptionService services.UserSubscriptionService
	conversionService   services.ConversionService
}

func NewSubscriptionIntegrationMiddleware(db *gorm.DB) SubscriptionIntegrationMiddleware {
	return &subscriptionIntegrationMiddleware{
		subscriptionService: services.NewSubscriptionService(db),
		conversionService:   services.NewConversionService(),
	}
}

// RequireSubscriptionForCourseCreation vérifie si l'utilisateur peut créer des cours
func (sim *subscriptionIntegrationMiddleware) RequireSubscriptionForCourseCreation() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			ctx.Next() // Sera géré par le middleware d'auth
			return
		}

		// Vérifier les limites de création de cours
		limitCheck, err := sim.subscriptionService.CheckUsageLimit(userId, "courses_created", 1)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to check course creation limit: " + err.Error(),
			})
			ctx.Abort()
			return
		}

		if !limitCheck.Allowed {
			// Préparer un message d'erreur avec suggestion d'upgrade
			message := limitCheck.Message
			if limitCheck.Limit > 0 {
				message += ". Upgrade your subscription to create more courses."
			} else {
				message = "Course creation requires an active subscription. Please upgrade your plan."
			}

			ctx.JSON(http.StatusPaymentRequired, &errors.APIError{
				ErrorCode:    http.StatusPaymentRequired,
				ErrorMessage: message,
			})
			ctx.Abort()
			return
		}

		// Stocker pour incrémenter après succès
		ctx.Set("increment_usage", map[string]any{
			"metric_type": "courses_created",
			"increment":   int64(1),
		})

		ctx.Next()

		// Incrémenter si la création a réussi
		if ctx.Writer.Status() == http.StatusCreated {
			sim.incrementUsageIfSuccessful(ctx, userId)
		}
	}
}

// RequireSubscriptionForAdvancedLabs vérifie l'accès aux labs avancés
func (sim *subscriptionIntegrationMiddleware) RequireSubscriptionForAdvancedLabs() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			ctx.Next()
			return
		}

		// Vérifier si l'utilisateur a accès aux labs avancés
		subscription, err := sim.subscriptionService.GetActiveUserSubscription(userId)
		if err != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Advanced labs require a premium subscription",
			})
			ctx.Abort()
			return
		}

		sPlan, errSPlan := sim.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
		if errSPlan != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Premium subscription required",
			})
			ctx.Abort()
			return
		}

		// Vérifier les fonctionnalités du plan
		features := sPlan.Features
		hasFeature := false
		for _, feature := range features {
			if feature == "advanced_labs" {
				hasFeature = true
				break
			}
		}

		if !hasFeature {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Your current plan doesn't include advanced labs. Please upgrade.",
			})
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// RequireSubscriptionForAPI vérifie l'accès à l'API
func (sim *subscriptionIntegrationMiddleware) RequireSubscriptionForAPI() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			ctx.Next()
			return
		}

		// Vérifier si c'est un appel API (via header ou path)
		isAPICall := ctx.GetHeader("X-API-Call") == "true" ||
			strings.HasPrefix(ctx.GetHeader("User-Agent"), "API/") ||
			ctx.Query("api") == "true"

		if !isAPICall {
			ctx.Next() // Pas un appel API
			return
		}

		subscription, err := sim.subscriptionService.GetActiveUserSubscription(userId)
		if err != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "API access requires an active subscription",
			})
			ctx.Abort()
			return
		}

		sPlan, errSPlan := sim.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
		if errSPlan != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Premium subscription required",
			})
			ctx.Abort()
			return
		}

		// Vérifier si le plan inclut l'accès API
		if !containsFeature(sPlan.Features, "api_access") {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Your current plan doesn't include API access. Please upgrade.",
			})
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// CheckSubscriptionRequirements middleware générique intelligent
func (sim *subscriptionIntegrationMiddleware) CheckSubscriptionRequirements() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")
		path := ctx.FullPath()
		method := ctx.Request.Method

		if userId == "" || method != "POST" {
			ctx.Next()
			return
		}

		// Déterminer les exigences selon le path
		var required bool
		var metricType string
		var featureRequired string
		var errorMessage string

		switch {
		case strings.Contains(path, "/courses") && method == "POST":
			required = true
			metricType = "courses_created"
			errorMessage = "Course creation limit exceeded. Upgrade for unlimited courses."

		case strings.Contains(path, "/terminals/start-session"):
			required = true
			featureRequired = "advanced_labs"
			errorMessage = "Advanced labs not included in your plan."

		case strings.Contains(path, "/generations") && method == "POST":
			required = true
			featureRequired = "export"
			errorMessage = "Course export requires a premium subscription."

		case strings.Contains(path, "/themes") && method == "POST":
			required = true
			featureRequired = "custom_themes"
			errorMessage = "Custom themes require a premium subscription."

		default:
			ctx.Next() // Pas de vérification nécessaire
			return
		}

		if !required {
			ctx.Next()
			return
		}

		// Récupérer l'abonnement
		subscription, err := sim.subscriptionService.GetActiveUserSubscription(userId)
		if err != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "This feature requires an active subscription",
			})
			ctx.Abort()
			return
		}

		sPlan, errSPlan := sim.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
		if errSPlan != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Active subscription required",
			})
			ctx.Abort()
			return
		}

		// Vérifier la fonctionnalité si nécessaire
		if featureRequired != "" {
			if !containsFeature(sPlan.Features, featureRequired) {
				ctx.JSON(http.StatusForbidden, &errors.APIError{
					ErrorCode:    http.StatusForbidden,
					ErrorMessage: errorMessage,
				})
				ctx.Abort()
				return
			}
		}

		// Vérifier les limites si nécessaire
		if metricType != "" {
			limitCheck, err := sim.subscriptionService.CheckUsageLimit(userId, metricType, 1)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, &errors.APIError{
					ErrorCode:    http.StatusInternalServerError,
					ErrorMessage: "Failed to check usage limit",
				})
				ctx.Abort()
				return
			}

			if !limitCheck.Allowed {
				ctx.JSON(http.StatusPaymentRequired, &errors.APIError{
					ErrorCode:    http.StatusPaymentRequired,
					ErrorMessage: errorMessage,
				})
				ctx.Abort()
				return
			}

			// Stocker pour incrémenter après succès
			ctx.Set("increment_usage", map[string]any{
				"metric_type": metricType,
				"increment":   int64(1),
			})
		}

		ctx.Next()

		// Incrémenter si succès
		if ctx.Writer.Status() >= 200 && ctx.Writer.Status() < 300 {
			sim.incrementUsageIfSuccessful(ctx, userId)
		}
	}
}

// InjectSubscriptionInfo injecte les infos d'abonnement dans le contexte
func (sim *subscriptionIntegrationMiddleware) InjectSubscriptionInfo() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId != "" {
			// Récupérer l'abonnement et l'injecter dans le contexte
			subscription, err := sim.subscriptionService.GetActiveUserSubscription(userId)

			sPlan, errSPlan := sim.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
			if errSPlan != nil {
				ctx.JSON(http.StatusForbidden, &errors.APIError{
					ErrorCode:    http.StatusForbidden,
					ErrorMessage: "Active subscription required",
				})
				ctx.Abort()
				return
			}

			if err == nil {
				ctx.Set("user_subscription", subscription)
				ctx.Set("subscription_plan", sPlan)
				ctx.Set("has_active_subscription", true)

				ctx.Set("user_features", sPlan.Features)
			} else {
				ctx.Set("has_active_subscription", false)
				ctx.Set("user_features", []string{}) // Aucune fonctionnalité premium
			}

			// Récupérer les métriques d'utilisation actuelles
			usage, err := sim.subscriptionService.GetUserUsageMetrics(userId)
			if err == nil {
				ctx.Set("user_usage", usage)
			}
		}

		ctx.Next()
	}
}

// incrementUsageIfSuccessful incrémente l'usage si stocké dans le contexte
func (sim *subscriptionIntegrationMiddleware) incrementUsageIfSuccessful(ctx *gin.Context, userId string) {
	usageInfo, exists := ctx.Get("increment_usage")
	if !exists {
		return
	}

	usageMap, ok := usageInfo.(map[string]any)
	if !ok {
		return
	}

	metricType, ok1 := usageMap["metric_type"].(string)
	increment, ok2 := usageMap["increment"].(int64)

	if ok1 && ok2 {
		err := sim.subscriptionService.IncrementUsage(userId, metricType, increment)
		if err != nil {
			// Log mais ne pas faire échouer la requête
			fmt.Printf("Warning: Failed to increment usage %s for user %s: %v\n", metricType, userId, err)
		}
	}
}

func containsFeature(features []string, targetFeature string) bool {
	for _, feature := range features {
		if feature == targetFeature {
			return true
		}
	}
	return false
}
