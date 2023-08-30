package userController

import (
	controller "soli/formations/src/auth/routes"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
	controller.GenericController
	service services.UserService
	config  *config.Configuration
}

func NewUserController(db *gorm.DB, userService services.UserService, config *config.Configuration) UserController {

	controller := &userController{
		GenericController: controller.NewGenericController(db),
		service:           userService,
		config:            config,
	}
	return controller
}
