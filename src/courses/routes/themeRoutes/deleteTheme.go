package themeController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete theme godoc
//
//	@Summary		Suppression theme
//	@Description	Suppression d'une theme dans la base de données
//	@Tags			themes
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID theme"
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Theme non trouvée - Impossible de le supprimer "
//
//	@Router			/themes/{id} [delete]
func (s themeController) DeleteTheme(ctx *gin.Context) {
	s.DeleteEntity(ctx, true)
}
