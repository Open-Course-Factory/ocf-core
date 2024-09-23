package scheduleController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func SchedulesRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	scheduleController := NewScheduleController(db)

	routes := router.Group("/schedules")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), scheduleController.GetSchedules)
	routes.GET("/:id", middleware.AuthManagement(), scheduleController.GetSchedule)
	routes.POST("", middleware.AuthManagement(), scheduleController.AddSchedule)
	routes.PATCH("/:id", middleware.AuthManagement(), scheduleController.EditSchedule)

	routes.DELETE("/:id", middleware.AuthManagement(), scheduleController.DeleteSchedule)
}
