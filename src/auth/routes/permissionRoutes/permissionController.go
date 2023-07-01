package permissionController

import (
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PermissionController interface {
	GetPermission(ctx *gin.Context)
	GetPermissions(ctx *gin.Context)
	AddPermission(ctx *gin.Context)
	EditPermission(ctx *gin.Context)
	DeletePermission(ctx *gin.Context)
}

type permissionController struct {
	service services.PermissionService
	config  *config.Configuration
}

func NewPermissionController(db *gorm.DB, config *config.Configuration) PermissionController {

	controller := &permissionController{
		service: services.NewPermissionService(db),
		config:  config,
	}
	return controller
}
