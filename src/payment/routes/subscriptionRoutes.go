// src/payment/routes/subscriptionRoutes.go
package paymentController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// SubscriptionRoutes définit les routes pour les abonnements
func SubscriptionRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	subscriptionController := NewSubscriptionController(db)
	authMiddleware := auth.NewAuthMiddleware(db)
	//usageLimitMiddleware := middleware.NewUsageLimitMiddleware(db)

	routes := router.Group("/subscriptions")

	// Routes spécialisées pour les abonnements utilisateur
	routes.POST("/checkout", authMiddleware.AuthManagement(), subscriptionController.CreateCheckoutSession)
	routes.POST("/portal", authMiddleware.AuthManagement(), subscriptionController.CreatePortalSession)
	routes.GET("/current", authMiddleware.AuthManagement(), subscriptionController.GetUserSubscription)
	routes.POST("/:id/cancel", authMiddleware.AuthManagement(), subscriptionController.CancelSubscription)
	routes.POST("/:id/reactivate", authMiddleware.AuthManagement(), subscriptionController.ReactivateSubscription)

	// Analytics (admin seulement)
	routes.GET("/analytics", authMiddleware.AuthManagement(), subscriptionController.GetSubscriptionAnalytics)

	// Usage monitoring
	routes.POST("/usage/check", authMiddleware.AuthManagement(), subscriptionController.CheckUsageLimit)
	routes.GET("/usage", authMiddleware.AuthManagement(), subscriptionController.GetUserUsage)
}
