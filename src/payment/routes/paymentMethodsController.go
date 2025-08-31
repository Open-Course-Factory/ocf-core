package paymentController

import (
	"net/http"
	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Payment Method Controller
type PaymentMethodController interface {
	AddEntity(ctx *gin.Context)
	DeleteEntity(ctx *gin.Context)
	GetEntities(ctx *gin.Context)

	SetDefaultPaymentMethod(ctx *gin.Context)
	GetUserPaymentMethods(ctx *gin.Context)
}

type paymentMethodController struct {
	controller.GenericController
	subscriptionService services.SubscriptionService
	stripeService       services.StripeService
}

func NewPaymentMethodController(db *gorm.DB) PaymentMethodController {
	return &paymentMethodController{
		GenericController:   controller.NewGenericController(db),
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
//	@Router			/payment-methods/{id}/set-default [post]
func (pmc *paymentMethodController) SetDefaultPaymentMethod(ctx *gin.Context) {
	userId := ctx.GetString("userId")
	paymentMethodID := ctx.Param("id")

	err := pmc.subscriptionService.SetDefaultPaymentMethod(userId, uuid.MustParse(paymentMethodID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
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
func (pmc *paymentMethodController) GetUserPaymentMethods(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	paymentMethods, err := pmc.subscriptionService.GetUserPaymentMethods(userId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, paymentMethods)
}

// Delete payment method godoc
//
//	@Summary		Suppression moyen de paiement
//	@Description	Suppression d'un moyen de paiement dans la base de données
//	@Tags			payment-methods
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID Payment Method"
//
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Méthode de paiement non trouvée - Impossible de la supprimer "
//
//	@Router			/payment-methods/{id} [delete]
func (pmc *paymentMethodController) DeleteEntity(ctx *gin.Context) {
	pmc.GenericController.DeleteEntity(ctx, true)
}
