package chapterController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetChapter godoc
//
//	@Summary		Récupération des chapters
//	@Description	Récupération de toutes les chapters disponibles
//	@Tags			chapters
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID chapters"
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.ChapterOutput
//
//	@Failure		404	{object}	errors.APIError	"Chapters inexistantes"
//
//	@Router			/chapters/{id} [get]
func (c chapterController) GetChapter(ctx *gin.Context) {
	c.GetEntity(ctx)
}
