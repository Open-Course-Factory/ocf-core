package connectionController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete connection godoc
//
//	@Summary		Suppression connection
//	@Description	Suppression d'une connection dans la base de données
//	@Tags			connections
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID connection"
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Connection non trouvée - Impossible de le supprimer "
//
//	@Router			/connections/{id} [delete]
func (s connectionController) DeleteConnection(ctx *gin.Context) {
	s.DeleteEntity(ctx, false)
}
