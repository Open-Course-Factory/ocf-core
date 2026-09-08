package controller

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"

	"gorm.io/gorm"
)

// HooksRoutes définit les routes pour la gestion des hooks (admin seulement)
func HooksRoutes(router *gin.RouterGroup, db *gorm.DB) {
	hooksController := NewGenericHooksController()
	authMiddleware := auth.NewAuthMiddleware(db)

	routes := router.Group("/hooks")

	// Routes pour la gestion des hooks (admin seulement)
	routes.GET("", authMiddleware.AuthManagement(), hooksController.ListHooks)
	routes.POST("/:hook_name/enable", authMiddleware.AuthManagement(), hooksController.EnableHook)
	routes.POST("/:hook_name/disable", authMiddleware.AuthManagement(), hooksController.DisableHook)
}
