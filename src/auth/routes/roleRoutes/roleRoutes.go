package roleController

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func RolesRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {

	roleController := NewRoleController(db, config)

	middleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	routes := router.Group("/roles")

	routes.POST("/", middleware.CheckIsLogged(), roleController.AddRole)
	routes.GET("/", middleware.CheckIsLogged(), roleController.GetRoles)
	routes.GET("/:id", middleware.CheckIsLogged(), roleController.GetRole)
	routes.DELETE("/:id", middleware.CheckIsLogged(), roleController.DeleteRole)
	routes.PUT("/:id", middleware.CheckIsLogged(), roleController.EditRole)
}
