package usernameController

import (
	controller "soli/formations/src/entityManagement/routes"
	services "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UsernameController interface {
	AddUsername(ctx *gin.Context)
	DeleteUsername(ctx *gin.Context)
	GetUsernames(ctx *gin.Context)
	GetUsername(ctx *gin.Context)
}

type usernameController struct {
	controller.GenericController
	service services.GenericService
}

func NewUsernameController(db *gorm.DB) UsernameController {
	return &usernameController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewGenericService(db),
	}
}
