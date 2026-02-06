// src/payment/middleware/usageLimitMiddleware.go
package middleware

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/services"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UsageLimitMiddleware interface {
	CheckCourseCreationLimit() gin.HandlerFunc
	CheckConcurrentUserLimit() gin.HandlerFunc
	CheckCustomLimit(metricType string, increment int64) gin.HandlerFunc
	CheckUsageForPath() gin.HandlerFunc // Middleware automatique basé sur le path

	// Terminal-specific middleware
	CheckTerminalCreationLimit() gin.HandlerFunc
	CheckConcurrentTerminalsLimit() gin.HandlerFunc
}

type usageLimitMiddleware struct {
	subscriptionService services.UserSubscriptionService
	orgSubService       services.OrganizationSubscriptionService
	terminalService     terminalServices.TerminalTrainerService
	db                  *gorm.DB
}

func NewUsageLimitMiddleware(db *gorm.DB) UsageLimitMiddleware {
	return &usageLimitMiddleware{
		subscriptionService: services.NewSubscriptionService(db),
		orgSubService:       services.NewOrganizationSubscriptionService(db),
		terminalService:     terminalServices.NewTerminalTrainerService(db),
		db:                  db,
	}
}

// CheckCourseCreationLimit vérifie la limite de création de cours
func (ulm *usageLimitMiddleware) CheckCourseCreationLimit() gin.HandlerFunc {
	return ulm.CheckCustomLimit("courses_created", 1)
}

// CheckConcurrentUserLimit vérifie la limite d'utilisateurs concurrents
func (ulm *usageLimitMiddleware) CheckConcurrentUserLimit() gin.HandlerFunc {
	return ulm.CheckCustomLimit("concurrent_users", 1)
}

// CheckCustomLimit middleware générique pour vérifier une limite spécifique
// Phase 2: Checks organization subscriptions first, then falls back to user subscriptions
func (ulm *usageLimitMiddleware) CheckCustomLimit(metricType string, increment int64) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			// Pas d'authentification, laisser passer (sera géré par le middleware d'auth)
			ctx.Next()
			return
		}

		// Phase 2: Try organization subscriptions first
		features, err := ulm.orgSubService.GetUserEffectiveFeatures(userId)
		var limitCheck *services.UsageLimitCheck

		if err == nil && features != nil {
			// User has organization subscriptions - use aggregated limits
			var limit int64
			switch metricType {
			case "courses_created":
				limit = int64(features.MaxCourses)
			case "concurrent_terminals":
				limit = int64(features.MaxConcurrentTerminals)
			default:
				limit = -1 // Unlimited
			}

			// For concurrent_terminals, check real count from DB
			var currentUsage int64 = 0
			if metricType == "concurrent_terminals" {
				var activeCount int64
				countErr := ulm.db.Table("terminals").
					Where("user_id = ? AND status = ? AND deleted_at IS NULL", userId, "active").
					Count(&activeCount).Error
				if countErr == nil {
					currentUsage = activeCount
				}
			}

			allowed := limit == -1 || (currentUsage+increment) <= limit
			var remaining int64
			if limit == -1 {
				remaining = -1
			} else {
				remaining = limit - currentUsage
				if remaining < 0 {
					remaining = 0
				}
			}

			message := ""
			if !allowed {
				message = fmt.Sprintf("Usage limit exceeded. Current: %d, Limit: %d", currentUsage, limit)
			}

			limitCheck = &services.UsageLimitCheck{
				Allowed:        allowed,
				CurrentUsage:   currentUsage,
				Limit:          limit,
				RemainingUsage: remaining,
				Message:        message,
				UserID:         userId,
				MetricType:     metricType,
			}
		} else {
			// Backward compatibility: Fall back to user subscription
			limitCheck, err = ulm.subscriptionService.CheckUsageLimit(userId, metricType, increment)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, &errors.APIError{
					ErrorCode:    http.StatusInternalServerError,
					ErrorMessage: fmt.Sprintf("Failed to check usage limit: %v", err),
				})
				ctx.Abort()
				return
			}
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
	subscriptionService services.UserSubscriptionService
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

		sPlan, errSPlan := urm.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
		if errSPlan != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Premium subscription required",
			})
			ctx.Abort()
			return
		}

		// Vérifier que c'est un plan premium (exemple de logique)
		isPremium := strings.Contains(strings.ToLower(sPlan.RequiredRole), "premium") ||
			sPlan.PriceAmount > 1000 // Plus de 10€/mois = premium

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
	subscriptionService services.UserSubscriptionService
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

// CheckTerminalCreationLimit vérifie les limites avant de créer un terminal
func (ulm *usageLimitMiddleware) CheckTerminalCreationLimit() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			ctx.Next()
			return
		}

		// Récupérer l'abonnement de l'utilisateur
		subscription, err := ulm.subscriptionService.GetActiveUserSubscription(userId)
		if err != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Active subscription required to create terminals",
			})
			ctx.Abort()
			return
		}

		// Récupérer le plan d'abonnement
		plan, err := ulm.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to retrieve subscription plan",
			})
			ctx.Abort()
			return
		}

		// Vérifier le nombre de terminaux actifs concurrents via CheckUsageLimit
		limitCheck, err := ulm.subscriptionService.CheckUsageLimit(userId, "concurrent_terminals", 1)
		if err != nil {
			log.Printf("[ERROR] CheckUsageLimit failed for user %s: %v", userId, err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to check concurrent terminal limit",
			})
			ctx.Abort()
			return
		}

		log.Printf("[DEBUG] Terminal limit check for user %s: allowed=%v, current=%d, limit=%d, remaining=%d, message=%s",
			userId, limitCheck.Allowed, limitCheck.CurrentUsage, limitCheck.Limit, limitCheck.RemainingUsage, limitCheck.Message)

		if !limitCheck.Allowed {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: fmt.Sprintf("Maximum concurrent terminals (%d) reached. Please stop a terminal or upgrade your plan.", plan.MaxConcurrentTerminals),
			})
			ctx.Abort()
			return
		}

		// Vérifier la RAM disponible sur le serveur Terminal Trainer
		// Utiliser nocache=true pour obtenir les données en temps réel
		metrics, err := ulm.terminalService.GetServerMetrics(true)
		if err != nil {
			// Log l'erreur mais ne pas bloquer la création si le service de métriques est indisponible
			fmt.Printf("Warning: Failed to check server metrics for terminal creation: %v\n", err)
		} else {
			// Calculer la RAM requise basée sur le type d'instance demandé
			// Map des tailles de machines vers RAM requise (GB)
			machineSizeToRAM := map[string]float64{
				"XS": 0.25,
				"S":  0.5,
				"M":  1.0,
				"L":  2.0,
				"XL": 4.0,
			}

			// Déterminer la taille de machine demandée depuis le corps de la requête
			// Pour éviter de lire le corps deux fois, on utilise une estimation par défaut
			// basée sur les tailles autorisées dans le plan
			var estimatedRAM float64 = 0.5 // valeur par défaut pour S

			// Utiliser la plus grande taille autorisée dans le plan comme estimation
			if len(plan.AllowedMachineSizes) > 0 {
				maxRAM := 0.0
				for _, size := range plan.AllowedMachineSizes {
					if size == "all" {
						estimatedRAM = 1.0 // Utiliser M comme moyenne pour "all"
						break
					}
					if ram, ok := machineSizeToRAM[size]; ok && ram > maxRAM {
						maxRAM = ram
					}
				}
				if maxRAM > 0 {
					estimatedRAM = maxRAM
				}
			}

			const minRAMReservePercent = 5.0

			// Calculer la RAM totale approximative à partir de la RAM disponible et du pourcentage utilisé
			// ram_available_gb = total_ram * (1 - ram_percent/100)
			// donc: total_ram = ram_available_gb / (1 - ram_percent/100)
			totalRAM := metrics.RAMAvailableGB / (1.0 - metrics.RAMPercent/100.0)
			minReservedRAM := totalRAM * (minRAMReservePercent / 100.0)

			// Vérifier qu'il reste assez de RAM après la création du terminal
			ramAfterCreation := metrics.RAMAvailableGB - estimatedRAM

			if ramAfterCreation < minReservedRAM {
				ctx.JSON(http.StatusServiceUnavailable, &errors.APIError{
					ErrorCode: http.StatusServiceUnavailable,
					ErrorMessage: fmt.Sprintf("Server at capacity: insufficient RAM available (%.2f GB available, %.2f GB required for terminal + %.2f GB reserve). Please try again later.",
						metrics.RAMAvailableGB, estimatedRAM, minReservedRAM),
				})
				ctx.Abort()
				return
			}

			// Stocker les métriques dans le contexte pour référence
			ctx.Set("server_metrics", metrics)
		}

		// Stocker le plan dans le contexte pour usage ultérieur
		ctx.Set("subscription_plan", plan)
		ctx.Next()

		// Si la requête a réussi (status 2xx), incrémenter concurrent_terminals
		if ctx.Writer.Status() >= 200 && ctx.Writer.Status() < 300 {
			err := ulm.subscriptionService.IncrementUsage(userId, "concurrent_terminals", 1)
			if err != nil {
				fmt.Printf("Warning: Failed to increment concurrent_terminals for user %s: %v\n", userId, err)
			}
		}
	}
}

// CheckConcurrentTerminalsLimit vérifie uniquement la limite de terminaux concurrents
func (ulm *usageLimitMiddleware) CheckConcurrentTerminalsLimit() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userId := ctx.GetString("userId")

		if userId == "" {
			ctx.Next()
			return
		}

		// Récupérer l'abonnement et le plan
		subscription, err := ulm.subscriptionService.GetActiveUserSubscription(userId)
		if err != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Active subscription required",
			})
			ctx.Abort()
			return
		}

		plan, err := ulm.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to retrieve subscription plan",
			})
			ctx.Abort()
			return
		}

		// Vérifier les terminaux concurrents via CheckUsageLimit
		limitCheck, err := ulm.subscriptionService.CheckUsageLimit(userId, "concurrent_terminals", 1)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to check concurrent terminal limit",
			})
			ctx.Abort()
			return
		}

		if !limitCheck.Allowed {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: fmt.Sprintf("Maximum concurrent terminals (%d) reached", plan.MaxConcurrentTerminals),
			})
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}
