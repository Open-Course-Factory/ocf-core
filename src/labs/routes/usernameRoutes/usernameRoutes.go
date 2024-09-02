package usernameController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func UsernamesRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	usernameController := NewUsernameController(db)

	routes := router.Group("/usernames")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), usernameController.GetUsernames)
	routes.GET("/:id", middleware.AuthManagement(), usernameController.GetUsername)
	routes.POST("", middleware.AuthManagement(), usernameController.AddUsername)
	routes.PATCH("/:id", middleware.AuthManagement(), usernameController.EditUsername)
	routes.DELETE("/:id", middleware.AuthManagement(), usernameController.DeleteUsername)
}
