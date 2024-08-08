package sshKeyController

import (
	dto "soli/formations/src/auth/dto"
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.CreateSshkeyOutput{}

// GetSshKeys godoc
//	@Summary		Récupération des sshKeys
//	@Description	Récupération de toutes les sshKeys disponibles
//	@Tags			sshKeys
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.SshkeyOutput
//
//	@Failure		404	{object}	errors.APIError	"SshKeys inexistantes"
//
//	@Router			/sshkeys [get]
func (s sshKeyController) GetSshKeys(ctx *gin.Context) {
	s.GetEntities(ctx)
}
