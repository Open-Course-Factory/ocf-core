// src/payment/routes/subscriptionPlanRoutes.go
package paymentController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// SubscriptionPlanRoutes définit les routes personnalisées pour les plans d'abonnement
// Les routes CRUD standards sont gérées automatiquement par le système d'entity management
func SubscriptionPlanRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	subscriptionController := NewSubscriptionController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	planRoutes := router.Group("/subscription-plans")

	// Routes de synchronisation Stripe (admin seulement)
	planRoutes.POST("/:id/sync-stripe", authMiddleware.AuthManagement(), subscriptionController.SyncSubscriptionPlanWithStripe)
	planRoutes.POST("/sync-stripe", authMiddleware.AuthManagement(), subscriptionController.SyncAllSubscriptionPlansWithStripe)
}