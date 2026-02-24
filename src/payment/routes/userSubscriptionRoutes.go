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

	// Read-only routes (no email verification required - needed for UI to display plan info)
	routes.GET("/current", authMw.AuthManagement(), subscriptionController.GetUserSubscription)
	routes.GET("/all", authMw.AuthManagement(), subscriptionController.GetAllUserSubscriptions)
	routes.GET("/usage", authMw.AuthManagement(), subscriptionController.GetUserUsage)

	// Payment actions require verified email
	routes.POST("/checkout", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.CreateCheckoutSession)
	routes.POST("/portal", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.CreatePortalSession)
	routes.POST("/:id/cancel", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.CancelSubscription)
	routes.POST("/:id/reactivate", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.ReactivateSubscription)
	routes.POST("/upgrade", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.UpgradeUserPlan)
	routes.POST("/usage/check", authMw.AuthManagement(), subscriptionController.CheckUsageLimit)
	routes.POST("/sync-usage-limits", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.SyncUsageLimits)

	// Analytics (admin only)
	routes.GET("/analytics", authMw.AuthManagement(), subscriptionController.GetSubscriptionAnalytics)

	// Admin subscription assignment (admin only)
	routes.POST("/admin-assign", authMw.AuthManagement(), subscriptionController.AdminAssignSubscription)

	// Subscription synchronization (admin only)
	routes.POST("/sync-existing", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.SyncExistingSubscriptions)
	routes.POST("/users/:user_id/sync", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.SyncUserSubscriptions)
	routes.POST("/sync-missing-metadata", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.SyncSubscriptionsWithMissingMetadata)
	routes.POST("/link/:subscription_id", authMw.AuthManagement(), verificationMw.RequireVerifiedEmail(), subscriptionController.LinkSubscriptionToUser)
}
