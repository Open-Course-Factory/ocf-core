package sshKeyController

import (
	"github.com/gin-gonic/gin"
)

// Delete sshKey godoc
//
// @Summary		Suppression sshKey
// @Description	Suppression d'une sshKey dans la base de données
// @Tags			sshKeys
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"ID sshKey"
//
// @Security Bearer
//
// @Success		204	{object}	string
//
// @Failure		400	{object}	errors.APIError	"Impossible de parser le json"
// @Failure		404	{object}	errors.APIError	"SshKey non trouvée - Impossible de le supprimer "
//
// @Router			/sshkeys/{id} [delete]
func (s sshKeyController) DeleteSshKey(ctx *gin.Context) {
	s.DeleteEntity(ctx)
}
