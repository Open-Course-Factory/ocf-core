package roleController

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func RolesRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {

	roleController := NewRoleController(db, config)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	permissionMiddleware := &middleware.PermissionsMiddleware{
		DB: db,
	}

	routes := router.Group("/roles")

	routes.POST("/", authMiddleware.CheckIsLogged(), roleController.AddRole)
	routes.GET("/", authMiddleware.CheckIsLogged(), roleController.GetRoles)
	routes.GET("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), roleController.GetRole)
	routes.DELETE("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), roleController.DeleteRole)
	routes.PUT("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), roleController.EditRole)
}
