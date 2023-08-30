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

	authMiddleware := &middleware.AuthMiddleware{
		DB:     db,
		Config: config,
	}

	genericService := services.NewGenericService(db)
	permissionMiddleware := middleware.NewPermissionsMiddleware(db, genericService)

	routes := router.Group("/users")

	routes.POST("/", userController.AddUser)

	routes.GET("/", authMiddleware.CheckIsLogged(), userController.GetUsers)
	routes.GET("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), userController.GetUser)
	routes.DELETE("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), userController.DeleteUser)
	routes.PATCH("/", authMiddleware.CheckIsLogged(), userController.EditUserSelf)
	routes.PUT("/:id", authMiddleware.CheckIsLogged(), permissionMiddleware.IsAuthorized(), userController.EditUser)

	routes.POST("/sshkey", authMiddleware.CheckIsLogged(), userController.AddUserSshKey)
}
