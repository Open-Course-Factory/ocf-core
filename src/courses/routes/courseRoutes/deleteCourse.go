package courseController

import (
	"net/http"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Delete course godoc
//
// @Summary		Suppression cours
// @Description	Suppression d'un cours dans la base de données
// @Tags			courses
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"ID cours"
//
// @Security Bearer
//
// @Success		204	{object}	string
//
// @Failure		400	{object}	errors.APIError	"Impossible de parser le json"
// @Failure		404	{object}	errors.APIError	"Cours non trouvé - Impossible de le supprimer "
//
// @Router			/courses/{id} [delete]
func (c courseController) DeleteCourse(ctx *gin.Context) {
	idParam := ctx.Param("id")

	id, parseError := uuid.Parse(idParam)
	if parseError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: parseError.Error(),
		})
		return
	}

	errorDelete := c.service.DeleteCourse(id)
	if errorDelete != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Course not found",
		})
		return
	}
	ctx.JSON(http.StatusNoContent, "Done")
}
