package sshKeyController

import (
	services "soli/formations/src/auth/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SshKeyController interface {
	AddSshKey(ctx *gin.Context)
	DeleteSshKey(ctx *gin.Context)
	GetSshKeys(ctx *gin.Context)
}

type sshKeyController struct {
	service services.SshKeyService
}

func NewSshKeyController(db *gorm.DB) SshKeyController {
	return &sshKeyController{
		service: services.NewSshKeyService(db),
	}
}
