package pageController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetPages godoc
//
//	@Summary		Récupération des page
//	@Description	Récupération de toutes les pages disponibles
//	@Tags			pages
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.PageOutput
//
//	@Failure		404	{object}	errors.APIError	"Page inexistante"
//
//	@Router			/pages [get]
func (c pageController) GetPages(ctx *gin.Context) {
	c.GetEntities(ctx)
}
