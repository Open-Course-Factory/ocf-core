package chapterController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.ChapterInput{}

// Add Chapter godoc
//
//	@Summary		Création chapter
//	@Description	Ajoute une nouvelle chapter dans la base de données
//	@Tags			chapters
//	@Accept			json
//	@Produce		json
//	@Param			chapter	body	dto.ChapterInput	true	"chapter"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.ChapterOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer une chapter"
//	@Failure		409	{object}	errors.APIError	"La chapter existe déjà"
//	@Router			/chapters [post]
func (s chapterController) AddChapter(ctx *gin.Context) {
	s.AddEntity(ctx)
}
