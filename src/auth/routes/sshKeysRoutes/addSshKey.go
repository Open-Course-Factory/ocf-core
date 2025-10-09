package sshKeyController

import (
	dto "soli/formations/src/auth/dto"
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.CreateSshKeyInput{}

// Add SshKey godoc
//
//	@Summary		Create SSH Key
//	@Description	Adds a new SSH key to the database
//	@Tags			ssh-keys
//	@Accept			json
//	@Produce		json
//	@Param			sshKey	body	dto.CreateSshKeyInput	true	"SSH Key"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.CreateSshKeyOutput
//
//	@Failure		400	{object}	errors.APIError	"Cannot parse JSON"
//	@Failure		400	{object}	errors.APIError	"Cannot create SSH key"
//	@Failure		409	{object}	errors.APIError	"SSH key already exists"
//	@Router			/ssh-keys [post]
func (s sshKeyController) AddSshKey(ctx *gin.Context) {
	s.AddEntity(ctx)
}
