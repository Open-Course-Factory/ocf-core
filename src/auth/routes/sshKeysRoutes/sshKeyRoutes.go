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

	routes.GET("", middleware.AuthManagement(), sshKeyController.GetSshkeys)
	routes.POST("", middleware.AuthManagement(), sshKeyController.AddSshkey)
	routes.PATCH("/:id", middleware.AuthManagement(), sshKeyController.EditSshkey)
	routes.DELETE("/:id", middleware.AuthManagement(), sshKeyController.DeleteSshkey)
}
