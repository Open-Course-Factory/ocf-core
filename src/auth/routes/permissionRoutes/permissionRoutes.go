package permissionController

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func PermissionsRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {

	permissionController := NewPermissionController(db, config)

	middleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	routes := router.Group("/permissions")

	routes.POST("/", middleware.CheckIsLogged(), permissionController.AddPermission)
	routes.GET("/", middleware.CheckIsLogged(), permissionController.GetPermissions)
	routes.GET("/:id", middleware.CheckIsLogged(), permissionController.GetPermission)
	routes.DELETE("/:id", middleware.CheckIsLogged(), permissionController.DeletePermission)
	routes.PUT("/:id", middleware.CheckIsLogged(), permissionController.EditPermission)
}
