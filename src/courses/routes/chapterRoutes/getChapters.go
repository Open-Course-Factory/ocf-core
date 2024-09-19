package chapterController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetChapters godoc
//
//	@Summary		Récupération des chapter
//	@Description	Récupération de toutes les chapters disponibles
//	@Tags			chapters
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.ChapterOutput
//
//	@Failure		404	{object}	errors.APIError	"Chapter inexistante"
//
//	@Router			/chapters [get]
func (c chapterController) GetChapters(ctx *gin.Context) {
	c.GetEntities(ctx)
}
