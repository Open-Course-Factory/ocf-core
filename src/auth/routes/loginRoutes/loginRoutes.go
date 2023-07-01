package loginController

import (
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func LoginRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	loginController := NewLoginController(db, config)

	routes := router.Group("/login")
	routes.POST("/", loginController.Login)
}

func RefreshRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	loginController := NewLoginController(db, config)

	routes := router.Group("/refresh")
	routes.POST("/", loginController.RefreshToken)
}
