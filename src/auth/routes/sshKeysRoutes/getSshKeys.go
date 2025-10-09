package sshKeyController

import (
	dto "soli/formations/src/auth/dto"
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.CreateSshKeyOutput{}

// GetSshKeys godoc
//
//	@Summary		Get SSH Keys
//	@Description	Retrieves all available SSH keys
//	@Tags			ssh-keys
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.SshKeyOutput
//
//	@Failure		404	{object}	errors.APIError	"SSH keys not found"
//
//	@Router			/ssh-keys [get]
func (s sshKeyController) GetSshKeys(ctx *gin.Context) {
	s.GetEntities(ctx)
}
