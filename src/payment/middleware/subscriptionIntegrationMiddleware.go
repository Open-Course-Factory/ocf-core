// src/payment/middleware/subscriptionIntegrationMiddleware.go
package middleware

import (
	"fmt"
	"net/http"
	"strconv"
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
	subscriptionService services.SubscriptionService
}

func NewSubscriptionIntegrationMiddleware(db *gorm.DB) SubscriptionIntegrationMiddleware {
	return &subscriptionIntegrationMiddleware{
		subscriptionService: services.NewSubscriptionService(db),
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
		ctx.Set("increment_usage", map[string]interface{}{
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

		// Vérifier les fonctionnalités du plan
		features := subscription.SubscriptionPlan.Features
		if !strings.Contains(features, "advanced_labs") {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Your current plan doesn't include advanced labs. Please upgrade.",
			})
			ctx.Abort()
			return
		}

		// Vérifier les limites de sessions
		limitCheck, err := sim.subscriptionService.CheckUsageLimit(userId, "lab_sessions", 1)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to check lab session limit: " + err.Error(),
			})
			ctx.Abort()
			return
		}

		if !limitCheck.Allowed {
			ctx.JSON(http.StatusPaymentRequired, &errors.APIError{
				ErrorCode:    http.StatusPaymentRequired,
				ErrorMessage: "Lab session limit exceeded. " + limitCheck.Message,
			})
			ctx.Abort()
			return
		}

		ctx.Set("increment_usage", map[string]interface{}{
			"metric_type": "lab_sessions",
			"increment":   int64(1),
		})

		ctx.Next()

		// Incrémenter si la session a été créée avec succès
		if ctx.Writer.Status() == http.StatusCreated || ctx.Writer.Status() == http.StatusOK {
			sim.incrementUsageIfSuccessful(ctx, userId)
		}
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

		// Vérifier si le plan inclut l'accès API
		features := subscription.SubscriptionPlan.Features
		if !strings.Contains(features, "api_access") {
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
			metricType = "lab_sessions"
			featureRequired = "advanced_labs"
			errorMessage = "Lab session limit exceeded or advanced labs not included in your plan."

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

		// Vérifier la fonctionnalité si nécessaire
		if featureRequired != "" {
			features := subscription.SubscriptionPlan.Features
			if !strings.Contains(features, featureRequired) {
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
			ctx.Set("increment_usage", map[string]interface{}{
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
			if err == nil {
				ctx.Set("user_subscription", subscription)
				ctx.Set("subscription_plan", subscription.SubscriptionPlan)
				ctx.Set("has_active_subscription", true)

				// Injecter les fonctionnalités disponibles
				features := strings.Split(subscription.SubscriptionPlan.Features, ",")
				ctx.Set("user_features", features)
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

	usageMap, ok := usageInfo.(map[string]interface{})
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

// RateLimitMiddleware basé sur l'abonnement
type SubscriptionBasedRateLimitMiddleware interface {
	ApplyRateLimit() gin.HandlerFunc
}

type subscriptionBasedRateLimitMiddleware struct {
	subscriptionService services.SubscriptionService
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

		var requestsPerMinute int
		if err != nil {
			// Utilisateur sans abonnement = limite très restrictive
			requestsPerMinute = 10
		} else {
			// Déterminer la limite selon le plan
			switch {
			case strings.Contains(subscription.SubscriptionPlan.RequiredRole, "enterprise"):
				requestsPerMinute = 1000
			case strings.Contains(subscription.SubscriptionPlan.RequiredRole, "organization"):
				requestsPerMinute = 500
			case strings.Contains(subscription.SubscriptionPlan.RequiredRole, "premium") ||
				strings.Contains(subscription.SubscriptionPlan.RequiredRole, "pro"):
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

		features := subscription.SubscriptionPlan.Features
		if !strings.Contains(features, featureName) {
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

		features := subscription.SubscriptionPlan.Features
		hasRequiredFeature := false

		for _, requiredFeature := range featuresRequired {
			if strings.Contains(features, requiredFeature) {
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

// ConcurrentUserLimitMiddleware pour limiter les utilisateurs concurrents
type ConcurrentUserLimitMiddleware interface {
	CheckConcurrentUsers() gin.HandlerFunc
}

type concurrentUserLimitMiddleware struct {
	subscriptionService services.SubscriptionService
}

func NewConcurrentUserLimitMiddleware(db *gorm.DB) ConcurrentUserLimitMiddleware {
	return &concurrentUserLimitMiddleware{
		subscriptionService: services.NewSubscriptionService(db),
	}
}

// CheckConcurrentUsers vérifie le nombre d'utilisateurs concurrents
func (cum *concurrentUserLimitMiddleware) CheckConcurrentUsers() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			ctx.Next()
			return
		}

		// Récupérer l'abonnement
		subscription, err := cum.subscriptionService.GetActiveUserSubscription(userId)
		if err != nil {
			// Utilisateur sans abonnement = accès limité à 1 utilisateur concurrent
			ctx.Set("max_concurrent_users", 1)
			ctx.Next()
			return
		}

		maxConcurrentUsers := subscription.SubscriptionPlan.MaxConcurrentUsers
		ctx.Set("max_concurrent_users", maxConcurrentUsers)

		// Vérifier le nombre d'utilisateurs actuellement connectés
		// (Cette logique dépend de votre système de session)
		// Pour l'instant, on stocke juste la limite dans le contexte

		ctx.Next()
	}
}

// StorageLimitMiddleware pour vérifier les limites de stockage
type StorageLimitMiddleware interface {
	CheckStorageLimit() gin.HandlerFunc
}

type storageLimitMiddleware struct {
	subscriptionService services.SubscriptionService
}

func NewStorageLimitMiddleware(db *gorm.DB) StorageLimitMiddleware {
	return &storageLimitMiddleware{
		subscriptionService: services.NewSubscriptionService(db),
	}
}

// CheckStorageLimit vérifie les limites de stockage pour les uploads
func (slm *storageLimitMiddleware) CheckStorageLimit() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" || ctx.Request.Method != "POST" {
			ctx.Next()
			return
		}

		// Vérifier si c'est un upload de fichier
		contentType := ctx.GetHeader("Content-Type")
		if !strings.Contains(contentType, "multipart/form-data") &&
			!strings.Contains(contentType, "application/octet-stream") {
			ctx.Next()
			return
		}

		// Récupérer la taille du contenu
		contentLengthStr := ctx.GetHeader("Content-Length")
		if contentLengthStr == "" {
			ctx.Next()
			return
		}

		contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64)
		if err != nil {
			ctx.Next()
			return
		}

		// Convertir en MB
		uploadSizeMB := contentLength / (1024 * 1024)

		// Vérifier l'abonnement et les limites
		subscription, err := slm.subscriptionService.GetActiveUserSubscription(userId)
		var storageLimit int64 = 10 // 10 MB pour les utilisateurs sans abonnement

		if err == nil {
			// TODO: Récupérer la limite de stockage depuis le plan
			// Pour l'instant, utiliser des valeurs par défaut selon le rôle
			role := subscription.SubscriptionPlan.RequiredRole
			switch {
			case strings.Contains(role, "enterprise"):
				storageLimit = -1 // Illimité
			case strings.Contains(role, "organization"):
				storageLimit = 20000 // 20 GB
			case strings.Contains(role, "premium") || strings.Contains(role, "pro"):
				storageLimit = 5000 // 5 GB
			default:
				storageLimit = 500 // 500 MB
			}
		}

		if storageLimit != -1 && uploadSizeMB > storageLimit {
			ctx.JSON(http.StatusPaymentRequired, &errors.APIError{
				ErrorCode:    http.StatusPaymentRequired,
				ErrorMessage: fmt.Sprintf("File too large. Your plan allows up to %d MB uploads", storageLimit),
			})
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}
