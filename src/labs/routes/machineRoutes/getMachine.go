package machineController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetMachine godoc
//
//	@Summary		Récupération des machine
//	@Description	Récupération de toutes les machine disponibles
//	@Tags			machines
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID machine"
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.MachineOutput
//
//	@Failure		404	{object}	errors.APIError	"Cours inexistants"
//
//	@Router			/machines/{id} [get]
func (c machineController) GetMachine(ctx *gin.Context) {
	c.GetEntity(ctx)
}
