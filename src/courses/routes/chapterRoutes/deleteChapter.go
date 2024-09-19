package chapterController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete chapter godoc
//
//	@Summary		Suppression chapter
//	@Description	Suppression d'une chapter dans la base de données
//	@Tags			chapters
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID chapter"
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Chapter non trouvée - Impossible de le supprimer "
//
//	@Router			/chapters/{id} [delete]
func (s chapterController) DeleteChapter(ctx *gin.Context) {
	s.DeleteEntity(ctx, true)
}
