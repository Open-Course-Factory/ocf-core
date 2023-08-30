package groupController

import (
	controller "soli/formations/src/auth/routes"
	"soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type GroupController interface {
	GetGroup(ctx *gin.Context)
	GetGroups(ctx *gin.Context)
	AddGroup(ctx *gin.Context)
	EditGroup(ctx *gin.Context)
	DeleteGroup(ctx *gin.Context)
}

type groupController struct {
	controller.GenericController
	service services.GroupService
}

func NewGroupController(db *gorm.DB) GroupController {

	controller := &groupController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewGroupService(db),
	}
	return controller
}
