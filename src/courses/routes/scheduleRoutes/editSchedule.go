package scheduleController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.EditScheduleInput{}

// Patch schedule godoc
//
//	@Summary		modification schedule
//	@Description	Modification d'une schedule dans la base de données
//	@Tags			schedules
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"ID schedule"
//	@Param			schedule	body	dto.EditScheduleInput	true	"Nouveau nom de la clé SSH"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Schedule non trouvée - Impossible de le modifier "
//
//	@Router			/schedules/{id} [patch]
func (s scheduleController) EditSchedule(ctx *gin.Context) {
	s.EditEntity(ctx)
}
