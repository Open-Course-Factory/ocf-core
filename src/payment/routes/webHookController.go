package paymentController

import (
	"net/http"
	"soli/formations/src/auth/errors"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Webhook Controller pour traiter les événements Stripe
type WebhookController interface {
	HandleStripeWebhook(ctx *gin.Context)
}

type webhookController struct {
	stripeService services.StripeService
}

func NewWebhookController(db *gorm.DB) WebhookController {
	return &webhookController{
		stripeService: services.NewStripeService(db),
	}
}

// Handle Stripe Webhook godoc
//
//	@Summary		Traiter les webhooks Stripe
//	@Description	Endpoint pour recevoir et traiter les événements de Stripe
//	@Tags			webhooks
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	string
//	@Failure		400	{object}	errors.APIError	"Invalid webhook"
//	@Router			/webhooks/stripe [post]
func (wc *webhookController) HandleStripeWebhook(ctx *gin.Context) {
	payload, err := ctx.GetRawData()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Failed to read request body",
		})
		return
	}

	signature := ctx.GetHeader("Stripe-Signature")
	if signature == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Missing Stripe signature",
		})
		return
	}

	// Traiter le webhook
	err = wc.stripeService.ProcessWebhook(payload, signature)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Webhook processing failed: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"received": true})
}
