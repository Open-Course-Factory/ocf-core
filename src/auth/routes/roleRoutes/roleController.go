package roleController

import (
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
	service services.RoleService
	config  *config.Configuration
}

func NewRoleController(db *gorm.DB, config *config.Configuration) RoleController {

	controller := &roleController{
		service: services.NewRoleService(db),
		config:  config,
	}
	return controller
}
