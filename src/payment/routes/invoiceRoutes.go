package paymentController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// InvoiceRoutes définit les routes pour les factures
func InvoiceRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	invoiceController := NewInvoiceController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/invoices")

	// Routes spécialisées
	routes.GET("/user", authMiddleware.AuthManagement(), invoiceController.GetUserInvoices)
	routes.POST("/sync", authMiddleware.AuthManagement(), invoiceController.SyncUserInvoices)
	routes.GET("/:id/download", authMiddleware.AuthManagement(), invoiceController.DownloadInvoice)

	// Admin routes
	routes.POST("/admin/cleanup", authMiddleware.AuthManagement(), invoiceController.CleanupInvoices)
}
