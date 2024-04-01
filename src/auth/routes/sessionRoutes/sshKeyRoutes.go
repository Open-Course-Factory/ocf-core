package sshKeyController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func SshKeysRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	sshKeyController := NewSshKeyController(db)

	routes := router.Group("/sshkeys")

	middleware := &auth.AuthMiddleware{}

	routes.GET("/", middleware.AuthManagement(), sshKeyController.GetSshKeys)
	routes.POST("/", middleware.AuthManagement(), sshKeyController.AddSshKey)

	routes.DELETE("/:id", middleware.AuthManagement(), sshKeyController.DeleteSshKey)
}
