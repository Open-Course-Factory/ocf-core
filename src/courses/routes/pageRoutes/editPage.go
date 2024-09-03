package pageController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.EditPageInput{}

// Patch page godoc
//
//	@Summary		modification page
//	@Description	Modification d'une page dans la base de données
//	@Tags			pages
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"ID page"
//	@Param			page	body	dto.EditPageInput	true	"Nouveau nom de la clé SSH"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Page non trouvée - Impossible de le modifier "
//
//	@Router			/pages/{id} [patch]
func (s pageController) EditPage(ctx *gin.Context) {
	s.EditEntity(ctx)
}
