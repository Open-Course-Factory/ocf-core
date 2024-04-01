package sessionController

import (
	"net/http"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Delete session godoc
//
// @Summary		Suppression session
// @Description	Suppression d'une session dans la base de données
// @Tags			sessions
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"ID session"
// @Security Bearer
//
// @Success		204	{object}	string
//
// @Failure		400	{object}	errors.APIError	"Impossible de parser le json"
// @Failure		404	{object}	errors.APIError	"Session non trouvée - Impossible de le supprimer "
//
// @Router			/sessions/{id} [delete]
func (s sessionController) DeleteSession(ctx *gin.Context) {
	idParam := ctx.Param("id")

	id, parseError := uuid.Parse(idParam)
	if parseError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: parseError.Error(),
		})
		return
	}

	errorDelete := s.service.DeleteSession(id)
	if errorDelete != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Session not found",
		})
		return
	}
	ctx.JSON(http.StatusNoContent, "Done")
}
