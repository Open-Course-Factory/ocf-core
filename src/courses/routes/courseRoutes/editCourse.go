package courseController

import (
	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.EditCourseInput{}

// Patch course godoc
//
//	@Summary		modification course
//	@Description	Modification d'une course dans la base de données
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"ID course"
//	@Param			course	body	dto.EditCourseInput	true	"Nouveau nom de la clé SSH"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Course non trouvée - Impossible de le modifier "
//
//	@Router			/courses/{id} [patch]
func (s courseController) EditCourse(ctx *gin.Context) {
	s.EditEntity(ctx)
}
