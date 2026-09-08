package authController

import (
	"github.com/gin-gonic/gin"

	"gorm.io/gorm"
)

func AuthRoutes(router *gin.RouterGroup, db *gorm.DB) {

	authController := NewAuthController()

	routes := router.Group("/auth")

	routes.GET("/callback", authController.Callback)
	routes.POST("/login", authController.Login)

}
