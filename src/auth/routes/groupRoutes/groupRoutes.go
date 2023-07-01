package groupController

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func GroupsRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	groupController := NewGroupController(db, config)

	middleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	routes := router.Group("/groups")

	routes.POST("/", middleware.CheckIsLogged(), groupController.AddGroup)
	routes.GET("/", middleware.CheckIsLogged(), groupController.GetGroups)
	routes.GET("/:id", middleware.CheckIsLogged(), groupController.GetGroup)
	routes.DELETE("/:id", middleware.CheckIsLogged(), groupController.DeleteGroup)
	routes.PUT("/:id", middleware.CheckIsLogged(), groupController.EditGroup)
}
