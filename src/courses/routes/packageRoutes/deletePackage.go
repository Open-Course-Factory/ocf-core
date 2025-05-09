package packageController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete package godoc
//
//	@Summary		Suppression package
//	@Description	Suppression d'une package dans la base de données
//	@Tags			packages
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID package"
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Package non trouvée - Impossible de le supprimer "
//
//	@Router			/packages/{id} [delete]
func (s packageController) DeletePackage(ctx *gin.Context) {
	s.DeleteEntity(ctx, true)
}
