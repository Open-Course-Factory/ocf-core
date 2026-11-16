// src/payment/routes/paymentMethodsController.go
package paymentController

import (
	goerrors "errors"
	"net/http"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Payment Method Controller
type PaymentMethodController struct {
	controller.GenericController
	subscriptionService services.UserSubscriptionService
	stripeService       services.StripeService
}

func NewPaymentMethodController(db *gorm.DB) *PaymentMethodController {
	return &PaymentMethodController{
		GenericController:   controller.NewGenericController(db, casdoor.Enforcer),
		subscriptionService: services.NewSubscriptionService(db),
		stripeService:       services.NewStripeService(db),
	}
}

// Set Default Payment Method godoc
//
//	@Summary		Définir le moyen de paiement par défaut
//	@Description	Définit un moyen de paiement comme celui par défaut pour l'utilisateur
//	@Tags			payment-methods
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Payment Method ID"
//	@Security		Bearer
//	@Success		200	{object}	string
//	@Failure		404	{object}	errors.APIError	"Payment method not found"
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		409	{object}	errors.APIError	"Payment method is not registered with Stripe"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/payment-methods/{id}/set-default [post]
func (pmc *PaymentMethodController) SetDefaultPaymentMethod(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	paymentMethodID := ctx.Param("id")

	parsedID, ok := parseUUIDParam(ctx, paymentMethodID, "Invalid payment method ID format")
	if !ok {
		return
	}

	methods, err := pmc.subscriptionService.GetUserPaymentMethods(userId)
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var stripePaymentMethodID string
	found := false
	for _, pm := range *methods {
		if pm.ID == parsedID {
			stripePaymentMethodID, found = pm.StripePaymentMethodID, true
			break
		}
	}
	if !found {
		errors.Respond(ctx, http.StatusNotFound, "Payment method not found")
		return
	}

	// Stripe first: the payment-method sync rewrites the local default from the Stripe customer.
	sub, err := pmc.subscriptionService.GetActiveUserSubscription(userId)
	if err != nil && !goerrors.Is(err, gorm.ErrRecordNotFound) {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if sub != nil && sub.StripeCustomerID != nil && *sub.StripeCustomerID != "" {
		if stripePaymentMethodID == "" {
			// A local-only method cannot be the Stripe default, and the sync would undo a local one.
			errors.Respond(ctx, http.StatusConflict, "Payment method is not registered with Stripe")
			return
		}
		if err := pmc.stripeService.SetDefaultPaymentMethod(*sub.StripeCustomerID, stripePaymentMethodID); err != nil {
			errors.Respond(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	}

	err = pmc.subscriptionService.SetDefaultPaymentMethod(userId, parsedID)
	if goerrors.Is(err, gorm.ErrRecordNotFound) {
		errors.Respond(ctx, http.StatusNotFound, "Payment method not found")
		return
	}
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Default payment method updated",
	})
}

// Get User Payment Methods godoc
//
//	@Summary		Récupérer les moyens de paiement de l'utilisateur
//	@Description	Retourne tous les moyens de paiement actifs de l'utilisateur connecté
//	@Tags			payment-methods
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{array}		dto.PaymentMethodOutput
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/payment-methods/user [get]
func (pmc *PaymentMethodController) GetUserPaymentMethods(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Récupérer depuis le service (retourne des models)
	paymentMethods, err := pmc.subscriptionService.GetUserPaymentMethods(userId)
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	// Convertir vers DTO
	paymentMethodsDTO := services.PaymentMethodsToDTO(paymentMethods)

	ctx.JSON(http.StatusOK, paymentMethodsDTO)
}

// Sync User Payment Methods godoc
//
//	@Summary		Synchroniser les moyens de paiement de l'utilisateur depuis Stripe
//	@Description	Récupère tous les moyens de paiement de Stripe et les synchronise dans la base de données locale
//	@Tags			payment-methods
//	@Accept			json
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	services.SyncPaymentMethodsResult
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/payment-methods/sync [post]
func (pmc *PaymentMethodController) SyncUserPaymentMethods(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	result, err := pmc.stripeService.SyncUserPaymentMethods(userId)
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	ctx.JSON(http.StatusOK, result)
}
