package courseController

import (
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

var _ = dto.CourseInput{}
var _ = dto.CourseOutput{}

// Add Course godoc
//
//	@Summary		Création cours
//	@Description	Ajoute un nouveau cours dans la base de données
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			course	body	dto.CourseInput	true	"cours"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.CourseOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer un cours"
//	@Failure		409	{object}	errors.APIError	"Le cours existe déjà pour cet utilisateur"
//	@Router			/courses [post]
func (c courseController) AddCourse(ctx *gin.Context) {
	c.AddEntity(ctx)
}
