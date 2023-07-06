package permissionAssociationController

import (
	"soli/formations/src/auth/services"
	config "soli/formations/src/configuration"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PermissionAssociationController interface {
	GetPermissionAssociation(ctx *gin.Context)
	GetPermissionAssociations(ctx *gin.Context)
	AddPermissionAssociation(ctx *gin.Context)
	// EditPermissionAssociation(ctx *gin.Context)
	DeletePermissionAssociation(ctx *gin.Context)
}

type permissionAssociationController struct {
	service services.PermissionAssociationService
	config  *config.Configuration
}

func NewPermissionAssociationController(db *gorm.DB, config *config.Configuration) PermissionAssociationController {

	controller := &permissionAssociationController{
		service: services.NewPermissionAssociationService(db),
		config:  config,
	}
	return controller
}
