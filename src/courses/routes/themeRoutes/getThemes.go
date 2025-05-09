package themeController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetThemes godoc
//
//	@Summary		Récupération des theme
//	@Description	Récupération de toutes les themes disponibles
//	@Tags			themes
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.ThemeOutput
//
//	@Failure		404	{object}	errors.APIError	"Theme inexistant"
//
//	@Router			/themes [get]
func (c themeController) GetThemes(ctx *gin.Context) {
	c.GetEntities(ctx)
}
