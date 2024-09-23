package scheduleController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetSchedules godoc
//
//	@Summary		Récupération des schedule
//	@Description	Récupération de toutes les schedules disponibles
//	@Tags			schedules
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.ScheduleOutput
//
//	@Failure		404	{object}	errors.APIError	"Schedule inexistante"
//
//	@Router			/schedules [get]
func (c scheduleController) GetSchedules(ctx *gin.Context) {
	c.GetEntities(ctx)
}
