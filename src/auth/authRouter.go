package authController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func AuthRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {

	authController := NewAuthController()

	routes := router.Group("/auth")

	routes.GET("/callback", authController.Callback)

}
