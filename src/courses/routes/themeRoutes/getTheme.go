package themeController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetTheme godoc
//
//	@Summary		Récupération des theme
//	@Description	Récupération de toutes les theme disponibles
//	@Tags			themes
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID theme"
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.ThemeOutput
//
//	@Failure		404	{object}	errors.APIError	"Theme inexistant"
//
//	@Router			/themes/{id} [get]
func (c themeController) GetTheme(ctx *gin.Context) {
	c.GetEntity(ctx)
}
