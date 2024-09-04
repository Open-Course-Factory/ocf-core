package machineController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetMachines godoc
//
//	@Summary		Récupération des machine
//	@Description	Récupération de toutes les machines disponibles
//	@Tags			machines
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.MachineOutput
//
//	@Failure		404	{object}	errors.APIError	"Machine inexistante"
//
//	@Router			/machines [get]
func (c machineController) GetMachines(ctx *gin.Context) {
	c.GetEntities(ctx)
}
