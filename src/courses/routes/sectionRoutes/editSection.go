package sectionController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.EditSectionInput{}

// Patch section godoc
//
//	@Summary		modification section
//	@Description	Modification d'une section dans la base de données
//	@Tags			sections
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"ID section"
//	@Param			section	body	dto.EditSectionInput	true	"Nouveau nom de la clé SSH"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Section non trouvée - Impossible de le modifier "
//
//	@Router			/sections/{id} [patch]
func (s sectionController) EditSection(ctx *gin.Context) {
	s.EditEntity(ctx)
}
