package machineController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/labs/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.MachineInput{}

// Add Machine godoc
//
//	@Summary		Création machine
//	@Description	Ajoute une nouvelle machine dans la base de données
//	@Tags			machines
//	@Accept			json
//	@Produce		json
//	@Param			machine	body	dto.MachineInput	true	"machine"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.MachineOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer une machine"
//	@Failure		409	{object}	errors.APIError	"La machine existe déjà"
//	@Router			/machines [post]
func (s machineController) AddMachine(ctx *gin.Context) {
	s.AddEntity(ctx)
}
