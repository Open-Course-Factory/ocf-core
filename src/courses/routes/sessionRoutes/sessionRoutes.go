package sessionController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func SessionsRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	sessionController := NewSessionController(db)

	routes := router.Group("/sessions")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), sessionController.GetSessions)
	routes.POST("", middleware.AuthManagement(), sessionController.AddSession)

	routes.DELETE("/:id", middleware.AuthManagement(), sessionController.DeleteSession)
}
