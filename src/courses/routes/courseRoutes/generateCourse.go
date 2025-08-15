// src/courses/routes/courseRoutes/generateCourse.go
package courseController

import (
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

// Generate Course godoc
//
//	@Summary		Génération d'un cours (mode asynchrone)
//	@Description	Génération d'un cours pour un format donné via le worker OCF
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			course	body	dto.GenerateCourseInput	true	"cours"
//
//	@Security		Bearer
//
//	@Success		202	{object}	dto.AsyncGenerationOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		500	{object}	errors.APIError	"Erreur lors de la génération"
//	@Router			/courses/generate [post]
func (c courseController) GenerateCourse(ctx *gin.Context) {
	courseGenerateDTO := dto.GenerateCourseInput{}

	bindError := ctx.BindJSON(&courseGenerateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json: " + bindError.Error(),
		})
		return
	}

	result, err := c.service.GenerateCourseAsync(courseGenerateDTO)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Erreur lors de la génération: " + err.Error(),
		})
		return
	}

	// Retourner un statut 202 (Accepted) pour indiquer le traitement asynchrone
	ctx.JSON(http.StatusAccepted, result)
}
