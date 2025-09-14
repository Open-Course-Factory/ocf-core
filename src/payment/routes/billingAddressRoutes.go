package paymentController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// BillingAddressRoutes définit les routes pour les adresses de facturation
func BillingAddressRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	billingController := NewBillingAddressController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/billing-addresses")

	// Routes spécialisées
	routes.GET("/user", authMiddleware.AuthManagement(), billingController.GetUserBillingAddresses)
	routes.POST("/:id/set-default", authMiddleware.AuthManagement(), billingController.SetDefaultBillingAddress)
}
