package pageController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete page godoc
//
//	@Summary		Suppression page
//	@Description	Suppression d'une page dans la base de données
//	@Tags			pages
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID page"
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Page non trouvée - Impossible de le supprimer "
//
//	@Router			/pages/{id} [delete]
func (s pageController) DeletePage(ctx *gin.Context) {
	s.DeleteEntity(ctx, true)
}
