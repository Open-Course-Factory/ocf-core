package connectionController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetConnection godoc
//
//	@Summary		Récupération des connection
//	@Description	Récupération de toutes les connection disponibles
//	@Tags			connections
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID connection"
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.ConnectionOutput
//
//	@Failure		404	{object}	errors.APIError	"Cours inexistants"
//
//	@Router			/connections/{id} [get]
func (c connectionController) GetConnection(ctx *gin.Context) {
	c.GetEntity(ctx)
}
