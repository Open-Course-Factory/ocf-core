package controller

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"

	"gorm.io/gorm"
)

func SshClientRoutes(router *gin.RouterGroup, db *gorm.DB) {
	sshClientController := NewSshClientController()

	routes := router.Group("/ssh")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), sshClientController.ShellWeb)

}
