package sshKeyController

import (
	services "soli/formations/src/auth/services"
	controller "soli/formations/src/entityManagement/routes"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SshKeyController interface {
	AddSshkey(ctx *gin.Context)
	EditSshkey(ctx *gin.Context)
	DeleteSshkey(ctx *gin.Context)
	GetSshkeys(ctx *gin.Context)
}

type sshKeyController struct {
	controller.GenericController
	service services.SshKeyService
}

func NewSshKeyController(db *gorm.DB) SshKeyController {
	return &sshKeyController{
		GenericController: controller.NewGenericController(db),
		service:           services.NewSshKeyService(db),
	}
}
