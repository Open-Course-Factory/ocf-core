package paymentController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// PaymentMethodRoutes définit les routes pour les moyens de paiement
func PaymentMethodRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	paymentMethodController := NewPaymentMethodController(db)
	authMiddleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/payment-methods")

	// Routes spécialisées
	routes.GET("/user", authMiddleware.AuthManagement(), paymentMethodController.GetUserPaymentMethods)
	routes.POST("/sync", authMiddleware.AuthManagement(), paymentMethodController.SyncUserPaymentMethods)
	routes.POST("/:id/set-default", authMiddleware.AuthManagement(), paymentMethodController.SetDefaultPaymentMethod)
}
