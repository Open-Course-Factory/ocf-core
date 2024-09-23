package scheduleController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetSchedule godoc
//
//	@Summary		Récupération des schedule
//	@Description	Récupération de toutes les schedule disponibles
//	@Tags			schedules
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID schedule"
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.ScheduleOutput
//
//	@Failure		404	{object}	errors.APIError	"Emploi du temps inexistant"
//
//	@Router			/schedules/{id} [get]
func (c scheduleController) GetSchedule(ctx *gin.Context) {
	c.GetEntity(ctx)
}
