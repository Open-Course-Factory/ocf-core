package paymentController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

// HooksRoutes définit les routes pour la gestion des hooks (admin seulement)
func HooksRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	hooksController := NewHooksController()
	authMiddleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/hooks")

	// Route spéciale pour Stripe
	routes.POST("/stripe/toggle", authMiddleware.AuthManagement(), hooksController.ToggleStripeSync)
}
