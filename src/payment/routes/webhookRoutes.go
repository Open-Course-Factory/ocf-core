package paymentController

import (
	"github.com/gin-gonic/gin"

	"gorm.io/gorm"
)

// WebhookRoutes définit les routes pour les webhooks (pas d'authentification)
func WebhookRoutes(router *gin.RouterGroup, db *gorm.DB) {
	webhookController := NewWebhookController(db)

	routes := router.Group("/webhooks")

	// Webhook Stripe (pas d'auth car Stripe appelle directement)
	routes.POST("/stripe", webhookController.HandleStripeWebhook)
}
