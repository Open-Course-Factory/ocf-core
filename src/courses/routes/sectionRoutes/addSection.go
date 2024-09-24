package sectionController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.SectionInput{}

// Add Section godoc
//
//	@Summary		Création section
//	@Description	Ajoute une nouvelle section dans la base de données
//	@Tags			sections
//	@Accept			json
//	@Produce		json
//	@Param			section	body	dto.SectionInput	true	"section"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.SectionOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer une section"
//	@Failure		409	{object}	errors.APIError	"La section existe déjà"
//	@Router			/sections [post]
func (s sectionController) AddSection(ctx *gin.Context) {
	s.AddEntity(ctx)
}
