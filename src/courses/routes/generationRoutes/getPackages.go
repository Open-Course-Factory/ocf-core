package generationController

import (
	errors "soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.GenerationOutput{}

// GetGenerations godoc
//
//	@Summary		Récupération des generations
//	@Description	Récupération de toutes les generations disponibles
//	@Tags			generations
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.GenerationOutput
//
//	@Failure		404	{object}	errors.APIError	"Generations inexistantes"
//
//	@Router			/generations [get]
func (s generationController) GetGenerations(ctx *gin.Context) {

	s.GetEntities(ctx)
}
