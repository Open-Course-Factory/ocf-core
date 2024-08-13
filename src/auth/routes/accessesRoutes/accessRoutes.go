package accessController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func AccessRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	accessController := NewAccessController()

	routes := router.Group("/accesses")

	middleware := auth.NewAuthMiddleware(db)

	routes.POST("", middleware.AuthManagement(), accessController.AddEntityAccesses)
	routes.DELETE("", middleware.AuthManagement(), accessController.DeleteEntityAccesses)
}
