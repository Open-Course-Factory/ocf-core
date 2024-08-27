package usernameController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete username godoc
//
//	@Summary		Suppression username
//	@Description	Suppression d'un username dans la base de données
//	@Tags			usernames
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID username"
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Username non trouvée - Impossible de le supprimer "
//
//	@Router			/usernames/{id} [delete]
func (s usernameController) DeleteUsername(ctx *gin.Context) {
	s.DeleteEntity(ctx, true)
}
