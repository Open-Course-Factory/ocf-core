package scheduleController

import (
	controller "soli/formations/src/entityManagement/routes"
	services "soli/formations/src/entityManagement/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ScheduleController interface {
	AddSchedule(ctx *gin.Context)
	DeleteSchedule(ctx *gin.Context)
	GetSchedules(ctx *gin.Context)
	GetSchedule(ctx *gin.Context)
	EditSchedule(ctx *gin.Context)
}

type scheduleController struct {
	controller.GenericController
	service services.GenericService
}

func NewScheduleController(db *gorm.DB) ScheduleController {
	return &scheduleController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewGenericService(db),
	}
}
