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

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", sshKeyController.GetSshKeys)
	// routes.GET("", middleware.AuthManagement(), sshKeyController.GetSshKeys)
	routes.POST("", sshKeyController.AddSshKey)
	// routes.POST("", middleware.AuthManagement(), sshKeyController.AddSshKey)
	//routes.PATCH("/:id", middleware.AuthManagement(), sshKeyController.PatchSshKeyName)
	routes.DELETE("/:id", middleware.AuthManagement(), sshKeyController.DeleteSshKey)
}
