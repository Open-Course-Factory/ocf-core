// src/payment/routes/subscriptionController.go
package paymentController

import (
	"fmt"
	"net/http"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/utils"
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
	GetAllUserSubscriptions(ctx *gin.Context)
	CancelSubscription(ctx *gin.Context)
	ReactivateSubscription(ctx *gin.Context)
	UpgradeUserPlan(ctx *gin.Context)
	GetSubscriptionAnalytics(ctx *gin.Context)
	CheckUsageLimit(ctx *gin.Context)
	GetUserUsage(ctx *gin.Context)

	// Pricing preview
	GetPricingPreview(ctx *gin.Context)

	// Méthodes pour la synchronisation Stripe des plans d'abonnement
	SyncSubscriptionPlanWithStripe(ctx *gin.Context)
	SyncAllSubscriptionPlansWithStripe(ctx *gin.Context)
	ImportPlansFromStripe(ctx *gin.Context)

	// Méthodes pour la synchronisation des abonnements existants
	SyncExistingSubscriptions(ctx *gin.Context)
	SyncUserSubscriptions(ctx *gin.Context)
	SyncSubscriptionsWithMissingMetadata(ctx *gin.Context)
	LinkSubscriptionToUser(ctx *gin.Context)

	// Utility methods
	SyncUsageLimits(ctx *gin.Context)
}

type userSubscriptionController struct {
	controller.GenericController
	db                  *gorm.DB
	subscriptionService services.UserSubscriptionService
	conversionService   services.ConversionService
	stripeService       services.StripeService
}

func NewSubscriptionController(db *gorm.DB) SubscriptionController {
	return &userSubscriptionController{
		GenericController:   controller.NewGenericController(db, casdoor.Enforcer),
		db:                  db,
		subscriptionService: services.NewSubscriptionService(db),
		conversionService:   services.NewConversionService(),
		stripeService:       services.NewStripeService(db),
	}
}

// Create Checkout Session godoc
//
//	@Summary		Créer une session de checkout Stripe ou un abonnement gratuit
//	@Description	Pour les plans payants, crée une session Stripe. Pour les plans gratuits (price=0), crée directement l'abonnement actif sans paiement. Le paramètre allow_replace=true permet de remplacer un abonnement gratuit existant par un abonnement payant.
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			checkout	body	dto.CreateCheckoutSessionInput	true	"Checkout session input (allow_replace: permet de remplacer un abonnement gratuit existant)"
//	@Security		Bearer
//	@Success		200	{object}	dto.CheckoutSessionOutput	"Paid plan: Stripe checkout URL"
//	@Success		200	{object}	map[string]any	"Free plan: {subscription: UserSubscriptionOutput, free_plan: true}"
//	@Failure		400	{object}	errors.APIError	"Bad request or user already has active subscription (use allow_replace=true to upgrade from free)"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		404	{object}	errors.APIError	"Plan not found"
//	@Failure		500	{object}	errors.APIError	"Stripe error"
//	@Router			/user-subscriptions/checkout [post]
func (sc *userSubscriptionController) CreateCheckoutSession(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	var input dto.CreateCheckoutSessionInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Check if user already has an active personal (non-assigned) subscription
	// For stacked subscriptions: assigned licenses don't block purchasing personal licenses
	existingSubscription, err := sc.subscriptionService.GetActiveUserSubscription(userId)
	var replaceSubscriptionID *uuid.UUID = nil

	if err == nil {
		// User has an active subscription - check if it's assigned or personal
		isAssigned := existingSubscription.SubscriptionBatchID != nil

		if isAssigned {
			// User has assigned license - allow purchasing personal license on top
			// The personal license will become primary, assigned stays as fallback
			utils.Info("User %s purchasing personal license while having assigned license", userId)
			// Don't set replaceSubscriptionID - we're stacking, not replacing
		} else {
			// User has personal subscription already
			if !input.AllowReplace {
				// No allow_replace flag - reject
				ctx.JSON(http.StatusBadRequest, &errors.APIError{
					ErrorCode:    http.StatusBadRequest,
					ErrorMessage: "User already has an active personal subscription",
				})
				return
			}

			// allow_replace is true - check if current subscription is free
			currentPlan, err := sc.subscriptionService.GetSubscriptionPlan(existingSubscription.SubscriptionPlanID)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, &errors.APIError{
					ErrorCode:    http.StatusInternalServerError,
					ErrorMessage: "Failed to get current subscription plan: " + err.Error(),
				})
				return
			}

			if currentPlan.PriceAmount > 0 {
				// Current plan is paid - don't allow replacement, require upgrade endpoint
				ctx.JSON(http.StatusBadRequest, &errors.APIError{
					ErrorCode:    http.StatusBadRequest,
					ErrorMessage: "Cannot replace paid personal subscription. Please use the upgrade endpoint instead.",
				})
				return
			}

			// Current plan is free - allow replacement
			utils.Info("User %s is upgrading from free plan (%s) to paid plan", userId, currentPlan.Name)
			replaceSubscriptionID = &existingSubscription.ID
		}
	}

	// Get the plan to check if it's free
	plan, err := sc.subscriptionService.GetSubscriptionPlan(input.SubscriptionPlanID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Subscription plan not found",
		})
		return
	}

	// FREE PLAN: Create subscription directly without Stripe
	if plan.PriceAmount == 0 {
		// CRITICAL: If user has an existing PAID subscription, cancel it in Stripe first
		if existingSubscription != nil && existingSubscription.StripeSubscriptionID != nil && *existingSubscription.StripeSubscriptionID != "" {
			currentPlan, _ := sc.subscriptionService.GetSubscriptionPlan(existingSubscription.SubscriptionPlanID)
			if currentPlan != nil && currentPlan.PriceAmount > 0 {
				// User is downgrading from paid to free - cancel Stripe subscription
				utils.Info("🔽 User %s downgrading from paid plan (%s) to free plan (%s) - canceling Stripe subscription",
					userId, currentPlan.Name, plan.Name)

				err := sc.stripeService.CancelSubscription(*existingSubscription.StripeSubscriptionID, false) // false = cancel immediately
				if err != nil {
					utils.Error("❌ Failed to cancel Stripe subscription %s: %v", existingSubscription.StripeSubscriptionID, err)
					ctx.JSON(http.StatusInternalServerError, &errors.APIError{
						ErrorCode:    http.StatusInternalServerError,
						ErrorMessage: "Failed to cancel existing Stripe subscription: " + err.Error(),
					})
					return
				}

				utils.Info("✅ Canceled Stripe subscription %s (webhook will handle database cleanup)", existingSubscription.StripeSubscriptionID)
			}
		}

		subscription, err := sc.subscriptionService.CreateUserSubscription(userId, input.SubscriptionPlanID)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to create free subscription: " + err.Error(),
			})
			return
		}

		// Convert to DTO and return
		subscriptionDTO, err := sc.conversionService.UserSubscriptionToDTO(subscription)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to convert subscription data",
			})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"subscription": subscriptionDTO,
			"free_plan":    true,
		})
		return
	}

	// PAID PLAN: Create Stripe checkout session
	checkoutSession, err := sc.stripeService.CreateCheckoutSession(userId, input, replaceSubscriptionID)
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
//	@Router			/user-subscriptions/portal [post]
func (sc *userSubscriptionController) CreatePortalSession(ctx *gin.Context) {
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
//	@Description	Retourne les détails de l'abonnement actif (prioritaire) de l'utilisateur connecté
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	dto.UserSubscriptionOutput
//	@Failure		404	{object}	errors.APIError	"No active subscription"
//	@Router			/user-subscriptions/current [get]
func (sc *userSubscriptionController) GetUserSubscription(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Récupérer l'abonnement depuis le service (retourne un model)
	subscription, err := sc.subscriptionService.GetActiveUserSubscription(userId)
	if err != nil {
		// This handles the case where user returns from checkout before webhook fires
		utils.Debug("No subscription found in DB for user %s, attempting sync from Stripe...", userId)

		syncResult, syncErr := sc.stripeService.SyncUserSubscriptions(userId)
		if syncErr != nil {
			utils.Debug("Failed to sync subscriptions for user %s: %v", userId, syncErr)
		} else if syncResult.CreatedSubscriptions > 0 || syncResult.UpdatedSubscriptions > 0 {
			utils.Debug("✅ Synced %d subscriptions for user %s",
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

	// Mark as primary (since this is the current/active endpoint)
	subscriptionDTO.IsPrimary = true

	ctx.JSON(http.StatusOK, subscriptionDTO)
}

// Get All User Subscriptions godoc
//
//	@Summary		Récupérer tous les abonnements actifs de l'utilisateur
//	@Description	Retourne tous les abonnements actifs (personnel + assigné) avec indication de celui qui est actif
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{array}		dto.UserSubscriptionOutput
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/user-subscriptions/all [get]
func (sc *userSubscriptionController) GetAllUserSubscriptions(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Get ALL active subscriptions
	subscriptions, err := sc.subscriptionService.GetAllActiveUserSubscriptions(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve subscriptions: " + err.Error(),
		})
		return
	}

	// If no subscriptions, return empty array
	if len(subscriptions) == 0 {
		ctx.JSON(http.StatusOK, []dto.UserSubscriptionOutput{})
		return
	}

	// Get the primary subscription to mark it
	primarySub, err := sc.subscriptionService.GetPrimaryUserSubscription(userId)
	var primaryID uuid.UUID
	if err == nil && primarySub != nil {
		primaryID = primarySub.ID
	}

	// Convert to DTOs and mark the primary one
	// Initialize as empty array instead of nil to ensure JSON returns [] instead of null
	subscriptionDTOs := make([]dto.UserSubscriptionOutput, 0)
	for _, sub := range subscriptions {
		subDTO, err := sc.conversionService.UserSubscriptionToDTO(&sub)
		if err != nil {
			utils.Warn("Failed to convert subscription %s: %v", sub.ID, err)
			continue
		}

		// Mark as primary if it's the highest priority subscription
		subDTO.IsPrimary = (sub.ID == primaryID)
		subscriptionDTOs = append(subscriptionDTOs, *subDTO)
	}

	ctx.JSON(http.StatusOK, subscriptionDTOs)
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
//	@Router			/user-subscriptions/{id}/cancel [post]
func (sc *userSubscriptionController) CancelSubscription(ctx *gin.Context) {
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

	// Check if this is a free subscription (no Stripe subscription ID)
	if subscription.StripeSubscriptionID == nil || *subscription.StripeSubscriptionID == "" {
		// Free subscription - cancel directly in our database
		updateErr := sc.stripeService.MarkSubscriptionAsCancelled(subscription)
		if updateErr != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to cancel free subscription: " + updateErr.Error(),
			})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"message": "Free subscription cancelled successfully",
		})
		return
	}

	// Paid subscription - cancel via Stripe
	err = sc.stripeService.CancelSubscription(*subscription.StripeSubscriptionID, !cancelImmediately)
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
			utils.Debug("⚠️ Warning: Failed to update DB after cancellation: %v", updateErr)
			// Don't fail the request - webhook will eventually update it
		} else {
			utils.Debug("✅ Subscription %s marked as cancelled in DB", *subscription.StripeSubscriptionID)
		}
	} else {
		// Cancel at period end - sync from Stripe to get the updated status
		_, syncErr := sc.stripeService.SyncUserSubscriptions(userId)
		if syncErr != nil {
			utils.Debug("⚠️ Warning: Failed to sync after cancellation: %v", syncErr)
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
//	@Router			/user-subscriptions/{id}/reactivate [post]
func (sc *userSubscriptionController) ReactivateSubscription(ctx *gin.Context) {
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

	// Check if this is a free subscription
	if subscription.StripeSubscriptionID == nil || *subscription.StripeSubscriptionID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Cannot reactivate free subscription via Stripe",
		})
		return
	}

	// Réactiver via Stripe
	err = sc.stripeService.ReactivateSubscription(*subscription.StripeSubscriptionID)
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

// Upgrade User Plan godoc
//
//	@Summary		Upgrade or change user's subscription plan
//	@Description	Upgrades or downgrades the user's subscription plan in Stripe with proration support and updates all usage metric limits atomically. Proration behavior options: "always_invoice" (default, immediate charge/credit), "create_prorations" (track but don't invoice immediately), "none" (no proration).
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			upgrade	body	dto.UpgradePlanInput	true	"Upgrade plan input (proration_behavior optional: always_invoice, create_prorations, none)"
//	@Security		Bearer
//	@Success		200	{object}	dto.UserSubscriptionOutput	"Subscription upgraded successfully"
//	@Failure		400	{object}	errors.APIError	"Bad request or new plan not configured in Stripe"
//	@Failure		404	{object}	errors.APIError	"No active subscription or plan not found"
//	@Failure		500	{object}	errors.APIError	"Internal server error or Stripe update failed"
//	@Router			/user-subscriptions/upgrade [post]
func (sc *userSubscriptionController) UpgradeUserPlan(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	var input dto.UpgradePlanInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Parse the new plan ID
	newPlanID, err := uuid.Parse(input.NewPlanID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid plan ID format",
		})
		return
	}

	// Get the current subscription to retrieve Stripe subscription ID
	currentSubscription, err := sc.subscriptionService.GetActiveUserSubscription(userId)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No active subscription found for user",
		})
		return
	}

	// Get the new plan to retrieve Stripe price ID
	newPlan, err := sc.subscriptionService.GetSubscriptionPlan(newPlanID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Subscription plan not found",
		})
		return
	}

	if newPlan.StripePriceID == nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "New plan does not have a Stripe price configured",
		})
		return
	}

	// Check if current subscription has a Stripe ID
	if currentSubscription.StripeSubscriptionID == nil || *currentSubscription.StripeSubscriptionID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Cannot upgrade free subscription via Stripe - please create a new paid subscription",
		})
		return
	}

	// Update the subscription in Stripe first
	_, err = sc.stripeService.UpdateSubscription(
		*currentSubscription.StripeSubscriptionID,
		*newPlan.StripePriceID,
		input.ProrationBehavior,
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to update subscription in Stripe: " + err.Error(),
		})
		return
	}

	// Update the plan in database (this updates both subscription and usage metric limits atomically)
	subscription, err := sc.subscriptionService.UpgradeUserPlan(userId, newPlanID, input.ProrationBehavior)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to upgrade plan in database: " + err.Error(),
		})
		return
	}

	// Convert to DTO
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
//	@Router			/user-subscriptions/analytics [get]
func (sc *userSubscriptionController) GetSubscriptionAnalytics(ctx *gin.Context) {
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
//	@Router			/user-subscriptions/usage/check [post]
func (sc *userSubscriptionController) CheckUsageLimit(ctx *gin.Context) {
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
//	@Router			/user-subscriptions/usage [get]
func (sc *userSubscriptionController) GetUserUsage(ctx *gin.Context) {
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

	// If no metrics found, return empty array instead of null
	if usageMetrics == nil || len(*usageMetrics) == 0 {
		ctx.JSON(http.StatusOK, []any{})
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
func (sc *userSubscriptionController) SyncSubscriptionPlanWithStripe(ctx *gin.Context) {
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
//	@Summary		Synchroniser tous les plans d'abonnement avec Stripe (DB → Stripe)
//	@Description	Crée les produits et prix Stripe pour tous les plans qui n'ont pas de StripePriceID
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	map[string]any	"Sync results"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/subscription-plans/sync-stripe [post]
func (sc *userSubscriptionController) SyncAllSubscriptionPlansWithStripe(ctx *gin.Context) {
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
	var failedPlans []map[string]any

	for _, plan := range plans {
		if plan.StripePriceID != nil {
			// Plan déjà synchronisé
			skippedPlans = append(skippedPlans, plan.Name+" (already synced)")
			continue
		}

		// Tenter de synchroniser le plan
		err := sc.stripeService.CreateSubscriptionPlanInStripe(&plan)
		if err != nil {
			failedPlans = append(failedPlans, map[string]any{
				"name":  plan.Name,
				"id":    plan.ID.String(),
				"error": err.Error(),
			})
		} else {
			syncedPlans = append(syncedPlans, plan.Name)
		}
	}

	ctx.JSON(http.StatusOK, map[string]any{
		"synced_plans":  syncedPlans,
		"skipped_plans": skippedPlans,
		"failed_plans":  failedPlans,
		"total_plans":   len(plans),
	})
}

// Import Plans From Stripe godoc
//
//	@Summary		Import subscription plans from Stripe (Stripe → DB)
//	@Description	Fetches all active products and prices from Stripe and creates/updates subscription plans in the database
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	services.SyncPlansResult	"Import results"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/subscription-plans/import-stripe [post]
func (sc *userSubscriptionController) ImportPlansFromStripe(ctx *gin.Context) {
	// Import plans from Stripe
	result, err := sc.stripeService.ImportPlansFromStripe()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to import plans from Stripe: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
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
//	@Router			/user-subscriptions/sync-existing [post]
func (sc *userSubscriptionController) SyncExistingSubscriptions(ctx *gin.Context) {
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
//	@Router			/user-subscriptions/users/{user_id}/sync [post]
func (sc *userSubscriptionController) SyncUserSubscriptions(ctx *gin.Context) {
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
//	@Router			/user-subscriptions/sync-missing-metadata [post]
func (sc *userSubscriptionController) SyncSubscriptionsWithMissingMetadata(ctx *gin.Context) {
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
//	@Success		200	{object}	map[string]any	"Success message"
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/user-subscriptions/link/{subscription_id} [post]
func (sc *userSubscriptionController) LinkSubscriptionToUser(ctx *gin.Context) {
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

	ctx.JSON(http.StatusOK, map[string]any{
		"message":           "Subscription linked successfully",
		"subscription_id":   subscriptionID,
		"user_id":           request.UserID,
		"subscription_plan": request.SubscriptionPlanID,
	})
}

// Delete entity (subscription plan)
func (sc *userSubscriptionController) DeleteEntity(ctx *gin.Context) {
	sc.GenericController.DeleteEntity(ctx, true)
}

// Sync Usage Limits godoc
//
//	@Summary		Sync usage limits with current subscription plan
//	@Description	Updates usage metric limits to match the user's current subscription plan (fixes desynchronization)
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	map[string]any	"Limits synced successfully"
//	@Failure		404	{object}	errors.APIError	"No active subscription"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/user-subscriptions/sync-usage-limits [post]
func (sc *userSubscriptionController) SyncUsageLimits(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Get active subscription
	subscription, err := sc.subscriptionService.GetActiveUserSubscription(userId)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No active subscription found",
		})
		return
	}

	// Update usage limits to match current plan
	err = sc.subscriptionService.UpdateUsageMetricLimits(userId, subscription.SubscriptionPlanID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to sync usage limits: " + err.Error(),
		})
		return
	}

	// Get updated metrics to return
	metrics, err := sc.subscriptionService.GetUserUsageMetrics(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to retrieve updated metrics",
		})
		return
	}

	metricsDTO, err := sc.conversionService.UsageMetricsListToDTO(metrics)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to convert metrics",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Usage limits synced successfully",
		"metrics": metricsDTO,
	})
}

// GetPricingPreview godoc
//
//	@Summary		Get pricing preview for bulk purchase
//	@Description	Calculate the detailed pricing breakdown for purchasing multiple licenses of a subscription plan, including volume discounts
//	@Tags			subscription-plans
//	@Accept			json
//	@Produce		json
//	@Param			subscription_plan_id	query		string	true	"Subscription Plan ID"
//	@Param			quantity				query		int		true	"Number of licenses"	minimum(1)
//	@Success		200						{object}	dto.PricingBreakdown
//	@Failure		400						{object}	errors.APIError	"Invalid parameters"
//	@Failure		404						{object}	errors.APIError	"Plan not found"
//	@Failure		500						{object}	errors.APIError	"Internal server error"
//	@Router			/subscription-plans/pricing-preview [get]
func (sc *userSubscriptionController) GetPricingPreview(ctx *gin.Context) {
	// Parse query parameters
	planIDStr := ctx.Query("subscription_plan_id")
	quantityStr := ctx.Query("quantity")

	if planIDStr == "" || quantityStr == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "subscription_plan_id and quantity are required",
		})
		return
	}

	planID, err := uuid.Parse(planIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid subscription_plan_id format",
		})
		return
	}

	var quantity int
	if _, err := fmt.Sscanf(quantityStr, "%d", &quantity); err != nil || quantity < 1 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid quantity (must be >= 1)",
		})
		return
	}

	// Create pricing service and calculate preview
	pricingService := services.NewPricingService(sc.db)
	preview, err := pricingService.CalculatePricingPreview(planID, quantity)
	if err != nil {
		utils.Error("Failed to calculate pricing preview: %v", err)
		if strings.Contains(err.Error(), "not found") {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Subscription plan not found",
			})
		} else {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to calculate pricing preview",
			})
		}
		return
	}

	ctx.JSON(http.StatusOK, preview)
}
