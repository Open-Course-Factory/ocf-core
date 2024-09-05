package chapterController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.EditChapterInput{}

// Patch chapter godoc
//
//	@Summary		modification chapter
//	@Description	Modification d'une chapter dans la base de données
//	@Tags			chapters
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"ID chapter"
//	@Param			chapter	body	dto.EditChapterInput	true	"Nouveau nom de la clé SSH"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Chapter non trouvée - Impossible de le modifier "
//
//	@Router			/chapters/{id} [patch]
func (s chapterController) EditChapter(ctx *gin.Context) {
	s.EditEntity(ctx)
}
