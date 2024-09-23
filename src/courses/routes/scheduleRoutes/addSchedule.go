package scheduleController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.ScheduleInput{}

// Add Schedule godoc
//
//	@Summary		Création schedule
//	@Description	Ajoute une nouvelle schedule dans la base de données
//	@Tags			schedules
//	@Accept			json
//	@Produce		json
//	@Param			schedule	body	dto.ScheduleInput	true	"schedule"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.ScheduleOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer une schedule"
//	@Failure		409	{object}	errors.APIError	"La schedule existe déjà"
//	@Router			/schedules [post]
func (s scheduleController) AddSchedule(ctx *gin.Context) {
	s.AddEntity(ctx)
}
