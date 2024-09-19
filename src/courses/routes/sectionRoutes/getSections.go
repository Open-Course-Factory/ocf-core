package sectionController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetSections godoc
//
//	@Summary		Récupération des section
//	@Description	Récupération de toutes les sections disponibles
//	@Tags			sections
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.SectionOutput
//
//	@Failure		404	{object}	errors.APIError	"Section inexistante"
//
//	@Router			/sections [get]
func (c sectionController) GetSections(ctx *gin.Context) {
	c.GetEntities(ctx)
}
