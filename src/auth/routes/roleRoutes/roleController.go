package roleController

import (
	controller "soli/formations/src/auth/routes"
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type RoleController interface {
	GetRole(ctx *gin.Context)
	GetRoles(ctx *gin.Context)
	AddRole(ctx *gin.Context)
	EditRole(ctx *gin.Context)
	DeleteRole(ctx *gin.Context)
}

type roleController struct {
	controller.GenericController
	service        services.RoleService
	genericService services.GenericService
	config         *config.Configuration
}

func NewRoleController(db *gorm.DB, config *config.Configuration) RoleController {

	controller := &roleController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewRoleService(db),
		genericService:    services.NewGenericService(db),
		config:            config,
	}
	return controller
}
