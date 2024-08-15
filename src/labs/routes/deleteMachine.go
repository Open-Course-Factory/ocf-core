package machineController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete machine godoc
//
//	@Summary		Suppression machine
//	@Description	Suppression d'une machine dans la base de données
//	@Tags			machines
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID machine"
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Machine non trouvée - Impossible de le supprimer "
//
//	@Router			/machines/{id} [delete]
func (s machineController) DeleteMachine(ctx *gin.Context) {
	s.DeleteEntity(ctx)
}
