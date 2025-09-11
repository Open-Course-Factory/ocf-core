// src/payment/routes/subscriptionController.go
package paymentController

import (
	"net/http"

	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SubscriptionController interface {
	// Méthodes génériques (héritées du système générique)
	AddEntity(ctx *gin.Context)
	EditEntity(ctx *gin.Context)
	DeleteEntity(ctx *gin.Context)
	GetEntities(ctx *gin.Context)
	GetEntity(ctx *gin.Context)

	// Méthodes spécialisées pour les abonnements
	CreateCheckoutSession(ctx *gin.Context)
	CreatePortalSession(ctx *gin.Context)
	GetUserSubscription(ctx *gin.Context)
	CancelSubscription(ctx *gin.Context)
	ReactivateSubscription(ctx *gin.Context)
	GetSubscriptionAnalytics(ctx *gin.Context)
	CheckUsageLimit(ctx *gin.Context)
	GetUserUsage(ctx *gin.Context)
}

type subscriptionController struct {
	controller.GenericController
	subscriptionService services.SubscriptionService
	conversionService   services.ConversionService
	stripeService       services.StripeService
}

func NewSubscriptionController(db *gorm.DB) SubscriptionController {
	return &subscriptionController{
		GenericController:   controller.NewGenericController(db),
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
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No active subscription found",
		})
		return
	}

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
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to cancel subscription: " + err.Error(),
		})
		return
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

// Delete entity (subscription plan)
func (sc *subscriptionController) DeleteEntity(ctx *gin.Context) {
	sc.GenericController.DeleteEntity(ctx, true)
}
