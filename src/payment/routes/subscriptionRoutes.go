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
	routes.POST("/upgrade", authMiddleware.AuthManagement(), subscriptionController.UpgradeUserPlan)

	// Analytics (admin seulement)
	routes.GET("/analytics", authMiddleware.AuthManagement(), subscriptionController.GetSubscriptionAnalytics)

	// Usage monitoring
	routes.POST("/usage/check", authMiddleware.AuthManagement(), subscriptionController.CheckUsageLimit)
	routes.GET("/usage", authMiddleware.AuthManagement(), subscriptionController.GetUserUsage)
	routes.POST("/sync-usage-limits", authMiddleware.AuthManagement(), subscriptionController.SyncUsageLimits)

	// Subscription synchronization (admin seulement)
	routes.POST("/sync-existing", authMiddleware.AuthManagement(), subscriptionController.SyncExistingSubscriptions)
	routes.POST("/users/:user_id/sync", authMiddleware.AuthManagement(), subscriptionController.SyncUserSubscriptions)
	routes.POST("/sync-missing-metadata", authMiddleware.AuthManagement(), subscriptionController.SyncSubscriptionsWithMissingMetadata)
	routes.POST("/link/:subscription_id", authMiddleware.AuthManagement(), subscriptionController.LinkSubscriptionToUser)
}
