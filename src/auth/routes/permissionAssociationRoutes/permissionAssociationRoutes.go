package permissionAssociationController

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func PermissionAssociationsRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {

	permissionAssociationController := NewPermissionAssociationController(db, config)

	middleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	routes := router.Group("/permissionAssociations")

	routes.POST("/", middleware.CheckIsLogged(), permissionAssociationController.AddPermissionAssociation)
	routes.GET("/", middleware.CheckIsLogged(), permissionAssociationController.GetPermissionAssociations)
	routes.GET("/:id", middleware.CheckIsLogged(), permissionAssociationController.GetPermissionAssociation)
	routes.DELETE("/:id", middleware.CheckIsLogged(), permissionAssociationController.DeletePermissionAssociation)
	routes.PUT("/:id", middleware.CheckIsLogged(), permissionAssociationController.EditPermissionAssociation)
}
