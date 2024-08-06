package groupController

import (
	services "soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
)

type GroupController interface {
	AddGroup(ctx *gin.Context)
	DeleteGroup(ctx *gin.Context)
	AddUserInGroup(ctx *gin.Context)
}

type groupController struct {
	service services.GroupService
}

func NewGroupController() GroupController {
	return &groupController{
		service: services.NewGroupService(),
	}
}
