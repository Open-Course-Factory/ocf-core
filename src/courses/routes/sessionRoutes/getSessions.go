package sessionController

import (
	errors "soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetSessions godoc
// @Summary Récupération des sessions
// @Description Récupération de toutes les sessions disponibles
// @Tags sessions
// @Accept json
// @Produce json
//
// @Security Bearer
//
// @Success 200 {object} []dto.SessionOutput
//
// @Failure 404 {object} errors.APIError "Sessions inexistantes"
//
// @Router /sessions [get]
func (s sessionController) GetSessions(ctx *gin.Context) {

	s.GetEntities(ctx)
}
