package scheduleController

import (
	errors "soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// Delete schedule godoc
//
//	@Summary		Suppression schedule
//	@Description	Suppression d'une schedule dans la base de données
//	@Tags			schedules
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID schedule"
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Schedule non trouvée - Impossible de le supprimer "
//
//	@Router			/schedules/{id} [delete]
func (s scheduleController) DeleteSchedule(ctx *gin.Context) {
	s.DeleteEntity(ctx, true)
}
