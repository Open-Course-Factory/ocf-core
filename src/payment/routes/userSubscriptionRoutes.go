// src/payment/routes/subscriptionRoutes.go
package paymentController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	authMiddleware "soli/formations/src/auth/middleware"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// UserSubscriptionRoutes définit les routes pour les abonnements
func UserSubscriptionRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	subscriptionController := NewSubscriptionController(db)
	authMw := auth.NewAuthMiddleware(db)
	verificationMiddleware := authMiddleware.NewEmailVerificationMiddleware(db)
	//usageLimitMiddleware := middleware.NewUsageLimitMiddleware(db)

	routes := router.Group("/user-subscriptions")
	// Require email verification for all subscription routes
	routes.Use(verificationMiddleware.RequireVerifiedEmail())

	// Routes spécialisées pour les abonnements utilisateur
	routes.POST("/checkout", authMw.AuthManagement(), subscriptionController.CreateCheckoutSession)
	routes.POST("/portal", authMw.AuthManagement(), subscriptionController.CreatePortalSession)
	routes.GET("/current", authMw.AuthManagement(), subscriptionController.GetUserSubscription)
	routes.GET("/all", authMw.AuthManagement(), subscriptionController.GetAllUserSubscriptions) // Get all active subscriptions
	routes.POST("/:id/cancel", authMw.AuthManagement(), subscriptionController.CancelSubscription)
	routes.POST("/:id/reactivate", authMw.AuthManagement(), subscriptionController.ReactivateSubscription)
	routes.POST("/upgrade", authMw.AuthManagement(), subscriptionController.UpgradeUserPlan)

	// Analytics (admin seulement)
	routes.GET("/analytics", authMw.AuthManagement(), subscriptionController.GetSubscriptionAnalytics)

	// Usage monitoring
	routes.POST("/usage/check", authMw.AuthManagement(), subscriptionController.CheckUsageLimit)
	routes.GET("/usage", authMw.AuthManagement(), subscriptionController.GetUserUsage)
	routes.POST("/sync-usage-limits", authMw.AuthManagement(), subscriptionController.SyncUsageLimits)

	// Subscription synchronization (admin seulement)
	routes.POST("/sync-existing", authMw.AuthManagement(), subscriptionController.SyncExistingSubscriptions)
	routes.POST("/users/:user_id/sync", authMw.AuthManagement(), subscriptionController.SyncUserSubscriptions)
	routes.POST("/sync-missing-metadata", authMw.AuthManagement(), subscriptionController.SyncSubscriptionsWithMissingMetadata)
	routes.POST("/link/:subscription_id", authMw.AuthManagement(), subscriptionController.LinkSubscriptionToUser)
}
