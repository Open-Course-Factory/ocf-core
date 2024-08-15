package sessionController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete session godoc
//
//	@Summary		Suppression session
//	@Description	Suppression d'une session dans la base de données
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID session"
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Session non trouvée - Impossible de le supprimer "
//
//	@Router			/sessions/{id} [delete]
func (s sessionController) DeleteSession(ctx *gin.Context) {
	s.DeleteEntity(ctx)
}
