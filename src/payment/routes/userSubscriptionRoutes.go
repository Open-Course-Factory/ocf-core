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
	verificationMw := authMiddleware.NewEmailVerificationMiddleware(db)

	routes := router.Group("/user-subscriptions")

	// Payment routes require verified email
	routes.POST("/checkout", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.CreateCheckoutSession)
	routes.POST("/portal", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.CreatePortalSession)
	routes.GET("/current", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.GetUserSubscription)
	routes.GET("/all", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.GetAllUserSubscriptions)
	routes.POST("/:id/cancel", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.CancelSubscription)
	routes.POST("/:id/reactivate", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.ReactivateSubscription)
	routes.POST("/upgrade", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.UpgradeUserPlan)

	// Analytics (admin seulement)
	routes.GET("/analytics", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.GetSubscriptionAnalytics)

	// Usage monitoring
	routes.POST("/usage/check", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.CheckUsageLimit)
	routes.GET("/usage", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.GetUserUsage)
	routes.POST("/sync-usage-limits", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.SyncUsageLimits)

	// Subscription synchronization (admin seulement)
	routes.POST("/sync-existing", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.SyncExistingSubscriptions)
	routes.POST("/users/:user_id/sync", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.SyncUserSubscriptions)
	routes.POST("/sync-missing-metadata", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.SyncSubscriptionsWithMissingMetadata)
	routes.POST("/link/:subscription_id", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.LinkSubscriptionToUser)
}
