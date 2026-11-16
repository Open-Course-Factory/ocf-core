package generationController

import (
	"net/http"

	errors "soli/formations/src/auth/errors"
	courseServices "soli/formations/src/courses/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	// a malformed id falls through to DeleteEntity's 400
	if id, err := uuid.Parse(ctx.Param("id")); err == nil {
		if _, err := courseServices.GetOwnedGeneration(s.service, id, ctx.GetString("userId"), ctx.GetStringSlice("userRoles")); errors.HandleError(http.StatusNotFound, err, ctx) {
			return
		}
	}
	s.DeleteEntity(ctx, true)
}
