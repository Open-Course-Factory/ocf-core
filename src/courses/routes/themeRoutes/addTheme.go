package themeController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.ThemeInput{}

// Add Theme godoc
//
//	@Summary		Création theme
//	@Description	Ajoute une nouvelle theme dans la base de données
//	@Tags			themes
//	@Accept			json
//	@Produce		json
//	@Param			theme	body	dto.ThemeInput	true	"theme"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.ThemeOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer une theme"
//	@Failure		409	{object}	errors.APIError	"La theme existe déjà"
//	@Router			/themes [post]
func (s themeController) AddTheme(ctx *gin.Context) {
	s.AddEntity(ctx)
}
