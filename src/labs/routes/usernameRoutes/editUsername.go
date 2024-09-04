package usernameController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/labs/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.EditUsernameInput{}

// Patch username godoc
//
//	@Summary		modification username
//	@Description	Modification d'une username dans la base de données
//	@Tags			usernames
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"ID username"
//	@Param			username	body	dto.EditUsernameInput	true	"Nouveau nom de la clé SSH"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Username non trouvée - Impossible de le modifier "
//
//	@Router			/usernames/{id} [patch]
func (s usernameController) EditUsername(ctx *gin.Context) {
	s.EditEntity(ctx)
}
