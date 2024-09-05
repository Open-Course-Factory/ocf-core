package courseController

import (
	"github.com/gin-gonic/gin"
)

// Add Course godoc
//
//	@Summary		Création cours
//	@Description	Ajoute un nouveau cours dans la base de données
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			course	body	dto.CreateCourseInput	true	"cours"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.CreateCourseOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer un cours"
//	@Failure		409	{object}	errors.APIError	"Le cours existe déjà pour cet utilisateur"
//	@Router			/courses [post]
func (c courseController) AddCourse(ctx *gin.Context) {
	c.AddEntity(ctx)
}
