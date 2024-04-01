package courseController

import (
	"net/http"

	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

// GetCourses godoc
// @Summary Récupération des cours
// @Description Récupération de tous les cours disponibles
// @Tags courses
// @Accept json
// @Produce json
//
// @Security Bearer
//
// @Success 200 {object} []dto.CourseOutput
//
// @Failure 404 {object} errors.APIError "Cours inexistants"
//
// @Router /courses [get]
func (c courseController) GetCourses(ctx *gin.Context) {

	// needed to keep dto in imports to make swagger find it
	var courses []dto.CourseOutput

	courses, err := c.service.GetCourses()

	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Courses not found",
		})
		return
	}

	ctx.JSON(http.StatusOK, courses)
}
