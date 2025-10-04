// src/payment/routes/subscriptionController.go
package paymentController

import (
	"soli/formations/src/auth/casdoor"
	"fmt"
	"net/http"
	"strings"

	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SubscriptionController interface {

	// Méthodes spécialisées pour les abonnements
	CreateCheckoutSession(ctx *gin.Context)
	CreatePortalSession(ctx *gin.Context)
	GetUserSubscription(ctx *gin.Context)
	CancelSubscription(ctx *gin.Context)
	ReactivateSubscription(ctx *gin.Context)
	GetSubscriptionAnalytics(ctx *gin.Context)
	CheckUsageLimit(ctx *gin.Context)
	GetUserUsage(ctx *gin.Context)

	// Méthodes pour la synchronisation Stripe des plans d'abonnement
	SyncSubscriptionPlanWithStripe(ctx *gin.Context)
	SyncAllSubscriptionPlansWithStripe(ctx *gin.Context)

	// Méthodes pour la synchronisation des abonnements existants
	SyncExistingSubscriptions(ctx *gin.Context)
	SyncUserSubscriptions(ctx *gin.Context)
	SyncSubscriptionsWithMissingMetadata(ctx *gin.Context)
	LinkSubscriptionToUser(ctx *gin.Context)
}

type subscriptionController struct {
	controller.GenericController
	subscriptionService services.SubscriptionService
	conversionService   services.ConversionService
	stripeService       services.StripeService
}

func NewSubscriptionController(db *gorm.DB) SubscriptionController {
	return &subscriptionController{
		GenericController:   controller.NewGenericController(db, casdoor.Enforcer),
		subscriptionService: services.NewSubscriptionService(db),
		conversionService:   services.NewConversionService(),
		stripeService:       services.NewStripeService(db),
	}
}

// Create Checkout Session godoc
//
//	@Summary		Créer une session de checkout Stripe
//	@Description	Initie le processus de paiement pour un abonnement
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			checkout	body	dto.CreateCheckoutSessionInput	true	"Checkout session input"
//	@Security		Bearer
//	@Success		200	{object}	dto.CheckoutSessionOutput
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		500	{object}	errors.APIError	"Stripe error"
//	@Router			/subscriptions/checkout [post]
func (sc *subscriptionController) CreateCheckoutSession(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	var input dto.CreateCheckoutSessionInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Vérifier si l'utilisateur n'a pas déjà un abonnement actif
	hasActive, err := sc.subscriptionService.HasActiveSubscription(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to check existing subscription: " + err.Error(),
		})
		return
	}

	if hasActive {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "User already has an active subscription",
		})
		return
	}

	// Créer la session de checkout
	checkoutSession, err := sc.stripeService.CreateCheckoutSession(userId, input)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to create checkout session: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, checkoutSession)
}

// Create Portal Session godoc
//
//	@Summary		Créer une session de portail client Stripe
//	@Description	Permet à l'utilisateur de gérer son abonnement
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			portal	body	dto.CreatePortalSessionInput	true	"Portal session input"
//	@Security		Bearer
//	@Success		200	{object}	dto.PortalSessionOutput
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"No active subscription"
//	@Router			/subscriptions/portal [post]
func (sc *subscriptionController) CreatePortalSession(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	var input dto.CreatePortalSessionInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Créer la session du portail
	portalSession, err := sc.stripeService.CreatePortalSession(userId, input)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, portalSession)
}

// Get User Subscription godoc
//
//	@Summary		Récupérer l'abonnement actif de l'utilisateur
//	@Description	Retourne les détails de l'abonnement actif de l'utilisateur connecté
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	dto.UserSubscriptionOutput
//	@Failure		404	{object}	errors.APIError	"No active subscription"
//	@Router			/subscriptions/current [get]
func (sc *subscriptionController) GetUserSubscription(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Récupérer l'abonnement depuis le service (retourne un model)
	subscription, err := sc.subscriptionService.GetActiveUserSubscription(userId)
	if err != nil {
		// This handles the case where user returns from checkout before webhook fires
		fmt.Printf("No subscription found in DB for user %s, attempting sync from Stripe...\n", userId)

		syncResult, syncErr := sc.stripeService.SyncUserSubscriptions(userId)
		if syncErr != nil {
			fmt.Printf("Failed to sync subscriptions for user %s: %v\n", userId, syncErr)
		} else if syncResult.CreatedSubscriptions > 0 || syncResult.UpdatedSubscriptions > 0 {
			fmt.Printf("✅ Synced %d subscriptions for user %s\n",
				syncResult.CreatedSubscriptions+syncResult.UpdatedSubscriptions, userId)

			// Retry getting the subscription after sync
			subscription, err = sc.subscriptionService.GetActiveUserSubscription(userId)
			if err == nil {
				// Success! Fall through to return subscription
				goto returnSubscription
			}
		}

		// Still no subscription found after sync attempt
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No active subscription found",
		})
		return
	}

returnSubscription:
	// Convertir vers DTO
	subscriptionDTO, err := sc.conversionService.UserSubscriptionToDTO(subscription)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to convert subscription data",
		})
		return
	}

	ctx.JSON(http.StatusOK, subscriptionDTO)
}

// Cancel Subscription godoc
//
//	@Summary		Annuler un abonnement
//	@Description	Annule l'abonnement actif (à la fin de la période ou immédiatement)
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			id					path	string	true	"Subscription ID"
//	@Param			cancel_immediately	query	bool	false	"Annuler immédiatement (default: false)"
//	@Security		Bearer
//	@Success		200	{object}	string
//	@Failure		404	{object}	errors.APIError	"Subscription not found"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/subscriptions/{id}/cancel [post]
func (sc *subscriptionController) CancelSubscription(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	subscriptionID := ctx.Param("id")
	cancelImmediately := ctx.Query("cancel_immediately") == "true"

	// Vérifier que l'abonnement appartient à l'utilisateur
	subscription, err := sc.subscriptionService.GetUserSubscriptionByID(uuid.MustParse(subscriptionID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Subscription not found",
		})
		return
	}

	if subscription.UserID != userId {
		userRoles := ctx.GetStringSlice("userRoles")
		isAdmin := false
		for _, role := range userRoles {
			if role == "administrator" {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Access denied to this subscription",
			})
			return
		}
	}

	// Annuler via Stripe
	err = sc.stripeService.CancelSubscription(subscription.StripeSubscriptionID, !cancelImmediately)
	if err != nil {
		// Vérifier si l'erreur indique que l'abonnement n'existe plus dans Stripe
		if strings.Contains(err.Error(), "resource_missing") || strings.Contains(err.Error(), "No such subscription") {
			// L'abonnement a déjà été supprimé dans Stripe, mettre à jour notre base de données
			updateErr := sc.stripeService.MarkSubscriptionAsCancelled(subscription)
			if updateErr != nil {
				ctx.JSON(http.StatusInternalServerError, &errors.APIError{
					ErrorCode:    http.StatusInternalServerError,
					ErrorMessage: "Failed to update subscription status: " + updateErr.Error(),
				})
				return
			}

			ctx.JSON(http.StatusOK, gin.H{
				"message": "Subscription was already cancelled and status updated",
			})
			return
		}

		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to cancel subscription: " + err.Error(),
		})
		return
	}

	// CRITICAL FIX: Update database immediately after successful Stripe cancellation
	// This prevents the need to wait for webhook delivery
	if cancelImmediately {
		// Immediate cancellation - mark as cancelled now
		updateErr := sc.stripeService.MarkSubscriptionAsCancelled(subscription)
		if updateErr != nil {
			fmt.Printf("⚠️ Warning: Failed to update DB after cancellation: %v\n", updateErr)
			// Don't fail the request - webhook will eventually update it
		} else {
			fmt.Printf("✅ Subscription %s marked as cancelled in DB\n", subscription.StripeSubscriptionID)
		}
	} else {
		// Cancel at period end - sync from Stripe to get the updated status
		_, syncErr := sc.stripeService.SyncUserSubscriptions(userId)
		if syncErr != nil {
			fmt.Printf("⚠️ Warning: Failed to sync after cancellation: %v\n", syncErr)
			// Don't fail the request - webhook will eventually update it
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Subscription cancelled successfully",
	})
}

// Reactivate Subscription godoc
//
//	@Summary		Réactiver un abonnement
//	@Description	Réactive un abonnement annulé (si possible)
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Subscription ID"
//	@Security		Bearer
//	@Success		200	{object}	string
//	@Failure		404	{object}	errors.APIError	"Subscription not found"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/subscriptions/{id}/reactivate [post]
func (sc *subscriptionController) ReactivateSubscription(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	subscriptionID := ctx.Param("id")

	// Vérifier l'accès
	subscription, err := sc.subscriptionService.GetUserSubscriptionByID(uuid.MustParse(subscriptionID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Subscription not found",
		})
		return
	}

	if subscription.UserID != userId {
		userRoles := ctx.GetStringSlice("userRoles")
		isAdmin := false
		for _, role := range userRoles {
			if role == "administrator" {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Access denied to this subscription",
			})
			return
		}
	}

	// Réactiver via Stripe
	err = sc.stripeService.ReactivateSubscription(subscription.StripeSubscriptionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to reactivate subscription: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Subscription reactivated successfully",
	})
}

// Get Subscription Analytics godoc
//
//	@Summary		Obtenir les analytics des abonnements
//	@Description	Retourne les statistiques et métriques des abonnements (admin seulement)
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			start_date	query	string	false	"Start date (YYYY-MM-DD)"
//	@Param			end_date	query	string	false	"End date (YYYY-MM-DD)"
//	@Security		Bearer
//	@Success		200	{object}	dto.SubscriptionAnalyticsOutput
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Router			/subscriptions/analytics [get]
func (sc *subscriptionController) GetSubscriptionAnalytics(ctx *gin.Context) {
	userRoles := ctx.GetStringSlice("userRoles")
	isAdmin := false
	for _, role := range userRoles {
		if role == "administrator" {
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Access denied - admin role required",
		})
		return
	}

	// Récupérer les analytics depuis le service (retourne un objet métier)
	analytics, err := sc.subscriptionService.GetSubscriptionAnalytics()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to get analytics: " + err.Error(),
		})
		return
	}

	// Convertir vers DTO
	analyticsDTO := sc.conversionService.SubscriptionAnalyticsToDTO(analytics)

	ctx.JSON(http.StatusOK, analyticsDTO)
}

// Check Usage Limit godoc
//
//	@Summary		Vérifier les limites d'utilisation
//	@Description	Vérifie si l'utilisateur peut effectuer une action selon ses limites d'abonnement
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			usage_check	body	dto.UsageLimitCheckInput	true	"Usage limit check"
//	@Security		Bearer
//	@Success		200	{object}	dto.UsageLimitCheckOutput
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Router			/subscriptions/usage/check [post]
func (sc *subscriptionController) CheckUsageLimit(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	var input dto.UsageLimitCheckInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Vérifier les limites via le service (retourne un objet métier)
	result, err := sc.subscriptionService.CheckUsageLimit(userId, input.MetricType, input.Increment)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Convertir vers DTO
	resultDTO := sc.conversionService.UsageLimitCheckToDTO(result)

	ctx.JSON(http.StatusOK, resultDTO)
}

// Get User Usage godoc
//
//	@Summary		Récupérer l'utilisation de l'utilisateur
//	@Description	Retourne toutes les métriques d'utilisation de l'utilisateur connecté
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{array}		dto.UsageMetricsOutput
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/subscriptions/usage [get]
func (sc *subscriptionController) GetUserUsage(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Récupérer les métriques depuis le service (retourne des models)
	usageMetrics, err := sc.subscriptionService.GetUserUsageMetrics(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Convertir vers DTO
	usageMetricsDTO, err := sc.conversionService.UsageMetricsListToDTO(usageMetrics)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to convert usage metrics",
		})
		return
	}

	ctx.JSON(http.StatusOK, usageMetricsDTO)
}

// Sync Subscription Plan with Stripe godoc
//
//	@Summary		Synchroniser un plan d'abonnement avec Stripe
//	@Description	Crée le produit et le prix Stripe pour un plan existant qui n'a pas de StripePriceID
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Subscription Plan ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.SubscriptionPlanOutput	"Plan synced successfully"
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"Plan not found"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/subscription-plans/{id}/sync-stripe [post]
func (sc *subscriptionController) SyncSubscriptionPlanWithStripe(ctx *gin.Context) {
	planIDStr := ctx.Param("id")
	planID, err := uuid.Parse(planIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid plan ID",
		})
		return
	}

	// Récupérer le plan
	plan, err := sc.subscriptionService.GetSubscriptionPlan(planID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Subscription plan not found",
		})
		return
	}

	// Vérifier si le plan a déjà un prix Stripe
	if plan.StripePriceID != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Plan already has a Stripe price configured",
		})
		return
	}

	// Créer le produit et prix dans Stripe
	err = sc.stripeService.CreateSubscriptionPlanInStripe(plan)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to sync plan with Stripe: " + err.Error(),
		})
		return
	}

	// Récupérer le plan mis à jour
	updatedPlan, err := sc.subscriptionService.GetSubscriptionPlan(planID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve updated plan",
		})
		return
	}

	// Convertir en DTO
	planDTO, err := sc.conversionService.SubscriptionPlanToDTO(updatedPlan)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to convert plan to DTO",
		})
		return
	}

	ctx.JSON(http.StatusOK, planDTO)
}

// Sync All Subscription Plans with Stripe godoc
//
//	@Summary		Synchroniser tous les plans d'abonnement avec Stripe
//	@Description	Crée les produits et prix Stripe pour tous les plans qui n'ont pas de StripePriceID
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	map[string]interface{}	"Sync results"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/subscription-plans/sync-stripe [post]
func (sc *subscriptionController) SyncAllSubscriptionPlansWithStripe(ctx *gin.Context) {
	// Récupérer tous les plans
	plansPtr, err := sc.subscriptionService.GetAllSubscriptionPlans(false) // Récupérer tous les plans, même inactifs
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve subscription plans: " + err.Error(),
		})
		return
	}

	plans := *plansPtr
	var syncedPlans []string
	var skippedPlans []string
	var failedPlans []map[string]interface{}

	for _, plan := range plans {
		if plan.StripePriceID != nil {
			// Plan déjà synchronisé
			skippedPlans = append(skippedPlans, plan.Name+" (already synced)")
			continue
		}

		// Tenter de synchroniser le plan
		err := sc.stripeService.CreateSubscriptionPlanInStripe(&plan)
		if err != nil {
			failedPlans = append(failedPlans, map[string]interface{}{
				"name":  plan.Name,
				"id":    plan.ID.String(),
				"error": err.Error(),
			})
		} else {
			syncedPlans = append(syncedPlans, plan.Name)
		}
	}

	ctx.JSON(http.StatusOK, map[string]interface{}{
		"synced_plans":  syncedPlans,
		"skipped_plans": skippedPlans,
		"failed_plans":  failedPlans,
		"total_plans":   len(plans),
	})
}

// Sync Existing Subscriptions godoc
//
//	@Summary		Synchroniser tous les abonnements Stripe existants
//	@Description	Récupère tous les abonnements depuis Stripe et les synchronise avec la base de données locale
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	services.SyncSubscriptionsResult	"Sync results"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/subscriptions/sync-existing [post]
func (sc *subscriptionController) SyncExistingSubscriptions(ctx *gin.Context) {
	// Synchroniser tous les abonnements depuis Stripe
	result, err := sc.stripeService.SyncExistingSubscriptions()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to sync existing subscriptions: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// Sync User Subscriptions godoc
//
//	@Summary		Synchroniser les abonnements d'un utilisateur spécifique
//	@Description	Récupère les abonnements d'un utilisateur depuis Stripe et les synchronise avec la base de données locale
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			user_id	path	string	true	"User ID"
//	@Security		Bearer
//	@Success		200	{object}	services.SyncSubscriptionsResult	"Sync results"
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/subscriptions/users/{user_id}/sync [post]
func (sc *subscriptionController) SyncUserSubscriptions(ctx *gin.Context) {
	userID := ctx.Param("user_id")
	if userID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "User ID is required",
		})
		return
	}

	// Synchroniser les abonnements de l'utilisateur depuis Stripe
	result, err := sc.stripeService.SyncUserSubscriptions(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to sync user subscriptions: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// Sync Subscriptions With Missing Metadata godoc
//
//	@Summary		Synchroniser les abonnements avec métadonnées manquantes
//	@Description	Tente de récupérer les métadonnées manquantes depuis les sessions de checkout et lie les abonnements aux utilisateurs
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	services.SyncSubscriptionsResult	"Sync results"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/subscriptions/sync-missing-metadata [post]
func (sc *subscriptionController) SyncSubscriptionsWithMissingMetadata(ctx *gin.Context) {
	// Synchroniser les abonnements avec métadonnées manquantes
	result, err := sc.stripeService.SyncSubscriptionsWithMissingMetadata()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to sync subscriptions with missing metadata: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// Link Subscription To User godoc
//
//	@Summary		Lier manuellement un abonnement à un utilisateur
//	@Description	Lie manuellement un abonnement Stripe spécifique à un utilisateur et un plan d'abonnement
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			subscription_id	path	string	true	"Stripe Subscription ID"
//	@Param			user_id	body	string	true	"User ID to link subscription to"
//	@Param			subscription_plan_id	body	string	true	"Subscription Plan ID"
//	@Security		Bearer
//	@Success		200	{object}	map[string]interface{}	"Success message"
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/subscriptions/link/{subscription_id} [post]
func (sc *subscriptionController) LinkSubscriptionToUser(ctx *gin.Context) {
	subscriptionID := ctx.Param("subscription_id")
	if subscriptionID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Subscription ID is required",
		})
		return
	}

	var request struct {
		UserID             string    `json:"user_id" binding:"required"`
		SubscriptionPlanID uuid.UUID `json:"subscription_plan_id" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Lier l'abonnement à l'utilisateur
	err := sc.stripeService.LinkSubscriptionToUser(subscriptionID, request.UserID, request.SubscriptionPlanID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to link subscription: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, map[string]interface{}{
		"message":           "Subscription linked successfully",
		"subscription_id":   subscriptionID,
		"user_id":           request.UserID,
		"subscription_plan": request.SubscriptionPlanID,
	})
}

// Delete entity (subscription plan)
func (sc *subscriptionController) DeleteEntity(ctx *gin.Context) {
	sc.GenericController.DeleteEntity(ctx, true)
}
