package sessionController

import (
	"net/http"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

// GetSessions godoc
// @Summary Récupération des sessions
// @Description Récupération de toutes les sessions disponibles
// @Tags sessions
// @Accept json
// @Produce json
//
// @Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
// @Success 200 {object} []dto.SessionOutput
//
// @Failure 404 {object} errors.APIError "Sessions inexistantes"
//
// @Router /sessions [get]
func (s sessionController) GetSessions(ctx *gin.Context) {

	var sessions []dto.SessionOutput
	sessions, err := s.service.GetSessions()

	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Sessions not found",
		})
		return
	}

	ctx.JSON(http.StatusOK, sessions)
}
