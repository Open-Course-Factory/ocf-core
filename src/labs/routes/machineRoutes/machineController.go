package machineController

import (
	"soli/formations/src/auth/casdoor"
	controller "soli/formations/src/entityManagement/routes"
	services "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MachineController interface {
	AddMachine(ctx *gin.Context)
	DeleteMachine(ctx *gin.Context)
	GetMachines(ctx *gin.Context)
	GetMachine(ctx *gin.Context)
	EditMachine(ctx *gin.Context)
}

type machineController struct {
	controller.GenericController
	service services.GenericService
}

func NewMachineController(db *gorm.DB) MachineController {
	return &machineController{
		GenericController: controller.NewGenericController(db, casdoor.Enforcer),
		service:           services.NewGenericService(db, casdoor.Enforcer),
	}
}
