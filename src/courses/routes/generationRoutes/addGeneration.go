package generationController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.GenerationInput{}

// Add Generation godoc
//
//	@Summary		Création generation
//	@Description	Ajoute une nouvelle generation dans la base de données
//	@Tags			generations
//	@Accept			json
//	@Produce		json
//	@Param			generation	body	dto.GenerationInput	true	"generation"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.GenerationOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer une generation"
//	@Failure		409	{object}	errors.APIError	"La generation existe déjà"
//	@Router			/generations [post]
func (s generationController) AddGeneration(ctx *gin.Context) {
	s.AddEntity(ctx)
}
