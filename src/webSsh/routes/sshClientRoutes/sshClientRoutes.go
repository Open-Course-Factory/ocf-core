package controller

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func SshClientRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	sshClientController := NewSshClientController()

	routes := router.Group("/ssh")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), sshClientController.ShellWeb)

}
