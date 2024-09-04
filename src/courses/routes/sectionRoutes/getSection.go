package sectionController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetSection godoc
//
//	@Summary		Récupération des sections
//	@Description	Récupération de toutes les sections disponibles
//	@Tags			sections
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID sections"
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.SectionOutput
//
//	@Failure		404	{object}	errors.APIError	"Sections inexistantes"
//
//	@Router			/sections/{id} [get]
func (c sectionController) GetSection(ctx *gin.Context) {
	c.GetEntity(ctx)
}
