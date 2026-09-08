package paymentController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"

	"gorm.io/gorm"
)

// HooksRoutes définit les routes pour la gestion des hooks (admin seulement)
func HooksRoutes(router *gin.RouterGroup, db *gorm.DB) {
	hooksController := NewHooksController()
	authMiddleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/hooks")

	// Route spéciale pour Stripe
	routes.POST("/stripe/toggle", authMiddleware.AuthManagement(), hooksController.ToggleStripeSync)
}
