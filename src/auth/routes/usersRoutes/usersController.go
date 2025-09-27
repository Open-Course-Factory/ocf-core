package userController

import (
	services "soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
)

type UserController interface {
	AddUser(ctx *gin.Context)
	DeleteUser(ctx *gin.Context)
	GetUsers(ctx *gin.Context)
	GetUser(ctx *gin.Context)
	GetUsersBatch(ctx *gin.Context)
	SearchUsers(ctx *gin.Context)
}

type userController struct {
	service services.UserService
}

func NewUserController() UserController {
	return &userController{
		service: services.NewUserService(),
	}
}
