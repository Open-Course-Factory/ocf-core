package sshKeyController

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.EditSshKeyInput{}

// Patch SSH Key godoc
//
//	@Summary		Update SSH Key
//	@Description	Updates an SSH key in the database
//	@Tags			ssh-keys
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"SSH Key ID"
//	@Param			sshKey	body	dto.EditSshKeyInput	true	"New SSH key name"
//
//	@Security		Bearer
//
//	@Success		200	{object}	dto.SshKeyOutput
//
//	@Failure		400	{object}	errors.APIError	"Cannot parse JSON"
//	@Failure		404	{object}	errors.APIError	"SSH key not found"
//
//	@Router			/ssh-keys/{id} [patch]
func (s sshKeyController) EditSshKey(ctx *gin.Context) {
	s.EditEntity(ctx)
}
