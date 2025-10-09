package sshKeyController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete SSH Key godoc
//
//	@Summary		Delete SSH Key
//	@Description	Deletes an SSH key from the database
//	@Tags			ssh-keys
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"SSH Key ID"
//
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Cannot parse JSON"
//	@Failure		404	{object}	errors.APIError	"SSH key not found"
//
//	@Router			/ssh-keys/{id} [delete]
func (s sshKeyController) DeleteSshKey(ctx *gin.Context) {
	s.DeleteEntity(ctx, true)
}
