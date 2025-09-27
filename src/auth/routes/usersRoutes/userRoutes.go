package userController

import (
	"github.com/gin-gonic/gin"

	auth "soli/formations/src/auth"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func UsersRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	userController := NewUserController()

	routes := router.Group("/users")

	middleware := auth.NewAuthMiddleware(db)

	// User CRUD routes
	routes.GET("", middleware.AuthManagement(), userController.GetUsers)
	routes.GET("/:id", middleware.AuthManagement(), userController.GetUser)
	routes.POST("", userController.AddUser)
	routes.DELETE("/:id", middleware.AuthManagement(), userController.DeleteUser)

	// User lookup routes for sharing functionality
	routes.POST("/batch", middleware.AuthManagement(), userController.GetUsersBatch)
	routes.GET("/search", middleware.AuthManagement(), userController.SearchUsers)
}
