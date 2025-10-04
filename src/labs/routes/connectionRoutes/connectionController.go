package connectionController

import (
	"soli/formations/src/auth/casdoor"
	controller "soli/formations/src/entityManagement/routes"
	services "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ConnectionController interface {
	AddConnection(ctx *gin.Context)
	DeleteConnection(ctx *gin.Context)
	GetConnections(ctx *gin.Context)
	GetConnection(ctx *gin.Context)
}

type connectionController struct {
	controller.GenericController
	service services.GenericService
}

func NewConnectionController(db *gorm.DB) ConnectionController {
	return &connectionController{
		GenericController: controller.NewGenericController(db, casdoor.Enforcer),
		service:           services.NewGenericService(db, casdoor.Enforcer),
	}
}
