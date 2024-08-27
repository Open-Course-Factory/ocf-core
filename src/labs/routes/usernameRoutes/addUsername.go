package usernameController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/labs/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.UsernameInput{}

// Add Username godoc
//
//	@Summary		Création username
//	@Description	Ajoute un nouvel username dans la base de données
//	@Tags			usernames
//	@Accept			json
//	@Produce		json
//	@Param			username	body	dto.UsernameInput	true	"username"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.UsernameOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer un username"
//	@Failure		409	{object}	errors.APIError	"La username existe déjà"
//	@Router			/usernames [post]
func (s usernameController) AddUsername(ctx *gin.Context) {
	s.AddEntity(ctx)
}
