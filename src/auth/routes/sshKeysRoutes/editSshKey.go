package sshKeyController

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.EditSshkeyInput{}

// Patch sshKey godoc
//
//	@Summary		modification sshKey
//	@Description	Modification d'une sshKey dans la base de données
//	@Tags			sshKeys
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"ID sshKey"
//	@Param			sshKey	body	dto.EditSshkeyInput	true	"Nouveau nom de la clé SSH"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"SshKey non trouvée - Impossible de le modifier "
//
//	@Router			/sshkeys/{id} [patch]
func (s sshKeyController) EditSshkey(ctx *gin.Context) {
	s.EditEntity(ctx)
}
