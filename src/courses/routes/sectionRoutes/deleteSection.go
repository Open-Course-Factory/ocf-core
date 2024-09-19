package sectionController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete section godoc
//
//	@Summary		Suppression section
//	@Description	Suppression d'une section dans la base de données
//	@Tags			sections
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID section"
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Section non trouvée - Impossible de le supprimer "
//
//	@Router			/sections/{id} [delete]
func (s sectionController) DeleteSection(ctx *gin.Context) {
	s.DeleteEntity(ctx, true)
}
