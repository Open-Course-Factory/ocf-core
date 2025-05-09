package generationController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete generation godoc
//
//	@Summary		Suppression generation
//	@Description	Suppression d'une generation dans la base de données
//	@Tags			generations
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID generation"
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Generation non trouvée - Impossible de le supprimer "
//
//	@Router			/generations/{id} [delete]
func (s generationController) DeleteGeneration(ctx *gin.Context) {
	s.DeleteEntity(ctx, true)
}
