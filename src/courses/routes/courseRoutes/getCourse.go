package courseController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetCourse godoc
//
//	@Summary		Récupération des courses
//	@Description	Récupération de toutes les courses disponibles
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID courses"
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.CourseOutput
//
//	@Failure		404	{object}	errors.APIError	"Courses inexistantes"
//
//	@Router			/courses/{id} [get]
func (c courseController) GetCourse(ctx *gin.Context) {
	c.GetEntity(ctx)
}
