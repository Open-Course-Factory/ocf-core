// src/payment/routes/bulkLicenseRoutes.go
package paymentController

import (
	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// BulkLicenseRoutes defines routes for bulk license management
func BulkLicenseRoutes(router *gin.RouterGroup, configuration *config.Configuration, db *gorm.DB) {
	bulkController := NewBulkLicenseController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	// Bulk purchase route (on user-subscriptions group)
	// Requires authentication + appropriate role (trainer, organization, group_manager, or admin)
	// NOTE: Role-based access, not subscription-based (you don't need a subscription to BUY one!)
	router.POST("/user-subscriptions/purchase-bulk",
		authMiddleware.AuthManagement(),
		bulkController.PurchaseBulkLicenses)

	// Batch management routes
	batchRoutes := router.Group("/subscription-batches")
	batchRoutes.Use(authMiddleware.AuthManagement())
	{
		batchRoutes.GET("", bulkController.GetMyBatches)                                          // List my batches
		batchRoutes.GET("/:id", bulkController.GetBatchDetails)                                   // Get batch details
		batchRoutes.GET("/:id/licenses", bulkController.GetBatchLicenses)                         // List licenses in batch
		batchRoutes.POST("/:id/assign", bulkController.AssignLicense)                             // Assign a license
		batchRoutes.DELETE("/:id/licenses/:license_id/revoke", bulkController.RevokeLicense)      // Revoke a license
		batchRoutes.PATCH("/:id/quantity", bulkController.UpdateBatchQuantity)                    // Update quantity
	}
}
