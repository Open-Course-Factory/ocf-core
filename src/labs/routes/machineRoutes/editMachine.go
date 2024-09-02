package machineController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/labs/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.EditMachineInput{}

// Patch machine godoc
//
//	@Summary		modification machine
//	@Description	Modification d'une machine dans la base de données
//	@Tags			machines
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"ID machine"
//	@Param			machine	body	dto.EditMachineInput	true	"Nouveau nom de la clé SSH"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Machine non trouvée - Impossible de le modifier "
//
//	@Router			/machines/{id} [patch]
func (s machineController) EditMachine(ctx *gin.Context) {
	s.EditEntity(ctx)
}
