package machineController

import (
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
}

type machineController struct {
	controller.GenericController
	service services.GenericService
}

func NewMachineController(db *gorm.DB) MachineController {
	return &machineController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewGenericService(db),
	}
}
