package paymentController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// WebhookRoutes définit les routes pour les webhooks (pas d'authentification)
func WebhookRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	webhookController := NewWebhookController(db)

	routes := router.Group("/webhooks")

	// Webhook Stripe (pas d'auth car Stripe appelle directement)
	routes.POST("/stripe", webhookController.HandleStripeWebhook)
}
