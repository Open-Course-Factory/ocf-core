package userController

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/middleware"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"

	"gorm.io/gorm"
)

func UsersRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	userService := services.NewUserService(db)
	userController := NewUserController(db, userService, config)

	middleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	routes := router.Group("/users")

	routes.POST("/", userController.AddUser)

	routes.GET("/", middleware.CheckIsLogged(), userController.GetUsers)
	routes.GET("/:id", middleware.CheckIsLogged(), userController.GetUser)
	routes.DELETE("/:id", middleware.CheckIsLogged(), userController.DeleteUser)
	routes.PATCH("/", middleware.CheckIsLogged(), userController.EditUserSelf)
	routes.PUT("/:id", middleware.CheckIsLogged(), userController.EditUser)

	routes.POST("/sshkey", middleware.CheckIsLogged(), userController.AddUserSshKey)
}
