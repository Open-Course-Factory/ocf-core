package connectionController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/labs/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.ConnectionInput{}

// Add Connection godoc
//
//	@Summary		Création connection
//	@Description	Ajoute une nouvelle connection dans la base de données
//	@Tags			connections
//	@Accept			json
//	@Produce		json
//	@Param			connection	body	dto.ConnectionInput	true	"connection"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.ConnectionOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer une connection"
//	@Failure		409	{object}	errors.APIError	"La connection existe déjà"
//	@Router			/connections [post]
func (s connectionController) AddConnection(ctx *gin.Context) {
	s.AddEntity(ctx)
}
