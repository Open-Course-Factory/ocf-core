// src/payment/routes/organizationSubscriptionRoutes.go
package paymentController

import (
	"github.com/gin-gonic/gin"
	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// OrganizationSubscriptionRoutes defines routes for organization subscriptions (Phase 2)
func OrganizationSubscriptionRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	orgSubController := NewOrganizationSubscriptionController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	// Organization subscription management routes
	orgRoutes := router.Group("/organizations/:id")
	orgRoutes.Use(authMiddleware.AuthManagement())
	{
		// Create organization subscription (POST /organizations/{id}/subscribe)
		orgRoutes.POST("/subscribe", orgSubController.CreateOrganizationSubscription)

		// Get organization subscription (GET /organizations/{id}/subscription)
		orgRoutes.GET("/subscription", orgSubController.GetOrganizationSubscription)

		// Cancel organization subscription (DELETE /organizations/{id}/subscription)
		orgRoutes.DELETE("/subscription", orgSubController.CancelOrganizationSubscription)

		// Get organization features (GET /organizations/{id}/features)
		orgRoutes.GET("/features", orgSubController.GetOrganizationFeatures)

		// Get organization usage limits (GET /organizations/{id}/usage-limits)
		orgRoutes.GET("/usage-limits", orgSubController.GetOrganizationUsageLimits)
	}

	// User feature access routes
	userRoutes := router.Group("/users/me")
	userRoutes.Use(authMiddleware.AuthManagement())
	{
		// Get user's effective features from all organizations
		// (GET /users/me/features)
		userRoutes.GET("/features", orgSubController.GetUserEffectiveFeatures)
	}

	// Admin bulk routes
	adminRoutes := router.Group("/admin/organizations")
	adminRoutes.Use(authMiddleware.AuthManagement())
	{
		adminRoutes.GET("/subscriptions", orgSubController.GetAllOrganizationSubscriptions)
	}
}
