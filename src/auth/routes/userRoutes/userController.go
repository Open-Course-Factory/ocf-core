package userController

import (
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
)

type UserController interface {
	GetUser(ctx *gin.Context)
	GetUsers(ctx *gin.Context)
	AddUser(ctx *gin.Context)
	EditUser(ctx *gin.Context)
	EditUserSelf(ctx *gin.Context)
	DeleteUser(ctx *gin.Context)
	AddUserSshKey(ctx *gin.Context)
}

type userController struct {
	service services.UserService
	config  *config.Configuration
}

func NewUserController(userService services.UserService, config *config.Configuration) UserController {

	controller := &userController{
		service: userService,
		config:  config,
	}
	return controller
}
