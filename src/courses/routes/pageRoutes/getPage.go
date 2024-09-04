package pageController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetPage godoc
//
//	@Summary		Récupération des page
//	@Description	Récupération de toutes les page disponibles
//	@Tags			pages
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID page"
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.PageOutput
//
//	@Failure		404	{object}	errors.APIError	"Cours inexistants"
//
//	@Router			/pages/{id} [get]
func (c pageController) GetPage(ctx *gin.Context) {
	c.GetEntity(ctx)
}
