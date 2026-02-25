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

	// User settings convenience routes
	routes.GET("/me/settings", middleware.AuthManagement(), userController.GetMySettings)
	routes.PATCH("/me/settings", middleware.AuthManagement(), userController.UpdateMySettings)
	routes.POST("/me/change-password", middleware.AuthManagement(), userController.ChangePassword)
	routes.POST("/me/force-change-password", middleware.AuthManagement(), userController.ForceChangePassword)

	// Auth/permission routes
	authRoutes := router.Group("/auth")
	authRoutes.GET("/me", middleware.AuthManagement(), GetCurrentUser)
	authRoutes.GET("/permissions", middleware.AuthManagement(), GetUserPermissions)
}
