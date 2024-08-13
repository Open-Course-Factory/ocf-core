package userController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func UsersRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	userController := NewUserController()

	routes := router.Group("/users")

	middleware := auth.NewAuthMiddleware(db)

	//routes.GET("", middleware.AuthManagement(), sshKeyController.GetSshKeys)
	routes.POST("", userController.AddUser)
	//routes.PATCH("/:id", middleware.AuthManagement(), sshKeyController.PatchSshKeyName)
	routes.DELETE("/:id", middleware.AuthManagement(), userController.DeleteUser)
}
