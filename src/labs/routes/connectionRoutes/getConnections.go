package connectionController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetConnections godoc
//
//	@Summary		Récupération des connection
//	@Description	Récupération de toutes les connections disponibles
//	@Tags			connections
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.ConnectionOutput
//
//	@Failure		404	{object}	errors.APIError	"Connection inexistante"
//
//	@Router			/connections [get]
func (c connectionController) GetConnections(ctx *gin.Context) {
	c.GetEntities(ctx)
}
