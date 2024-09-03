package pageController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.PageInput{}

// Add Page godoc
//
//	@Summary		Création page
//	@Description	Ajoute une nouvelle page dans la base de données
//	@Tags			pages
//	@Accept			json
//	@Produce		json
//	@Param			page	body	dto.PageInput	true	"page"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.PageOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer une page"
//	@Failure		409	{object}	errors.APIError	"La page existe déjà"
//	@Router			/pages [post]
func (s pageController) AddPage(ctx *gin.Context) {
	s.AddEntity(ctx)
}
