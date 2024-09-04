package sshKeyController

import (
	dto "soli/formations/src/auth/dto"
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.CreateSshkeyInput{}

// Add SshKey godoc
//
//	@Summary		Création sshKey
//	@Description	Ajoute une nouvelle sshKey dans la base de données
//	@Tags			sshKeys
//	@Accept			json
//	@Produce		json
//	@Param			sshKey	body	dto.CreateSshkeyInput	true	"sshKey"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.CreateSshkeyOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer une sshKey"
//	@Failure		409	{object}	errors.APIError	"La sshKey existe déjà"
//	@Router			/sshkeys [post]
func (s sshKeyController) AddSshkey(ctx *gin.Context) {
	s.AddEntity(ctx)
}
