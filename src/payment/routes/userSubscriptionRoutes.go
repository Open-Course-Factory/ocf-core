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

	// Routes spécialisées pour les abonnements utilisateur
	// Email verification is checked AFTER auth middleware to ensure userId is in context
	routes.POST("/checkout", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.CreateCheckoutSession)
	routes.POST("/portal", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.CreatePortalSession)
	routes.GET("/current", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.GetUserSubscription)
	routes.GET("/all", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.GetAllUserSubscriptions) // Get all active subscriptions
	routes.POST("/:id/cancel", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.CancelSubscription)
	routes.POST("/:id/reactivate", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.ReactivateSubscription)
	routes.POST("/upgrade", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.UpgradeUserPlan)

	// Analytics (admin seulement)
	routes.GET("/analytics", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.GetSubscriptionAnalytics)

	// Usage monitoring
	routes.POST("/usage/check", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.CheckUsageLimit)
	routes.GET("/usage", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.GetUserUsage)
	routes.POST("/sync-usage-limits", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.SyncUsageLimits)

	// Subscription synchronization (admin seulement)
	routes.POST("/sync-existing", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.SyncExistingSubscriptions)
	routes.POST("/users/:user_id/sync", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.SyncUserSubscriptions)
	routes.POST("/sync-missing-metadata", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.SyncSubscriptionsWithMissingMetadata)
	routes.POST("/link/:subscription_id", authMw.AuthManagement(), verificationMiddleware.RequireVerifiedEmail(), subscriptionController.LinkSubscriptionToUser)
}
