package themeController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.EditThemeInput{}

// Patch theme godoc
//
//	@Summary		modification theme
//	@Description	Modification d'une theme dans la base de données
//	@Tags			themes
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"ID theme"
//	@Param			theme	body	dto.EditThemeInput	true	"données du theme"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Theme non trouvée - Impossible de le modifier "
//
//	@Router			/themes/{id} [patch]
func (s themeController) EditTheme(ctx *gin.Context) {
	s.EditEntity(ctx)
}
