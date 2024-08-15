package courseController

import (
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}

// GetCourses godoc
//
//	@Summary		Récupération des cours
//	@Description	Récupération de tous les cours disponibles
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	[]dto.CourseOutput
//
//	@Failure		404	{object}	errors.APIError	"Cours inexistants"
//
//	@Router			/courses [get]
func (c courseController) GetCourses(ctx *gin.Context) {
	c.GetEntities(ctx)
}
