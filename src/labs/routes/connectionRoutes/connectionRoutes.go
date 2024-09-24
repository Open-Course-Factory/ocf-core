package connectionController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func ConnectionsRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	connectionController := NewConnectionController(db)

	routes := router.Group("/connections")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), connectionController.GetConnections)
	routes.GET("/:id", middleware.AuthManagement(), connectionController.GetConnection)
	routes.POST("", middleware.AuthManagement(), connectionController.AddConnection)

	routes.DELETE("/:id", middleware.AuthManagement(), connectionController.DeleteConnection)
}
