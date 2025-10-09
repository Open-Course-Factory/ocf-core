package sshKeyController

import (
	"soli/formations/src/auth/casdoor"
	services "soli/formations/src/auth/services"
	controller "soli/formations/src/entityManagement/routes"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SshKeyController interface {
	AddSshKey(ctx *gin.Context)
	EditSshKey(ctx *gin.Context)
	DeleteSshKey(ctx *gin.Context)
	GetSshKeys(ctx *gin.Context)
}

type sshKeyController struct {
	controller.GenericController
	service services.SshKeyService
}

func NewSshKeyController(db *gorm.DB) SshKeyController {
	return &sshKeyController{
		GenericController: controller.NewGenericController(db, casdoor.Enforcer),
		service:           services.NewSshKeyService(db),
	}
}
