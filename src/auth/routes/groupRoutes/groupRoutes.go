package groupController

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func GroupsRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	groupController := NewGroupController(db)

	authMiddleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	genericService := services.NewGenericService(db)
	permissionMiddleware := middleware.NewPermissionsMiddleware(db, genericService)

	routes := router.Group("/groups")

	routes.POST("/", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), groupController.AddGroup)
	routes.GET("/", authMiddleware.CheckIsLogged(), groupController.GetGroups)
	routes.GET("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), groupController.GetGroup)
	routes.DELETE("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), groupController.DeleteGroup)
	routes.PUT("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), groupController.EditGroup)
}
