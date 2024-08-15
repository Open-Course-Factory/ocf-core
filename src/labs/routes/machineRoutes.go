package machineController

import (
	"github.com/gin-gonic/gin"

	config "soli/formations/src/configuration"

	"gorm.io/gorm"

	auth "soli/formations/src/auth"
)

func MachinesRoutes(router *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	machineController := NewMachineController(db)

	routes := router.Group("/machines")

	middleware := auth.NewAuthMiddleware(db)

	routes.GET("", middleware.AuthManagement(), machineController.GetMachines)
	routes.GET("/:id", middleware.AuthManagement(), machineController.GetMachine)
	routes.POST("", middleware.AuthManagement(), machineController.AddMachine)

	routes.DELETE("/:id", middleware.AuthManagement(), machineController.DeleteMachine)
}
