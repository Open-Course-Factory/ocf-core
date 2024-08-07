package groupController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func GroupRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	groupController := NewGroupController()

	routes := router.Group("/groups")

	middleware := auth.NewAuthMiddleware(db)

	routes.POST("", middleware.AuthManagement(), groupController.AddGroup)
	routes.PATCH("/:name", middleware.AuthManagement(), groupController.ModifyUsersInGroup)
	routes.DELETE("/:name", middleware.AuthManagement(), groupController.DeleteGroup)
}
