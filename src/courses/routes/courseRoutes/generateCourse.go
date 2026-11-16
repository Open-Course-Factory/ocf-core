// src/courses/routes/courseRoutes/generateCourse.go
package courseController

import (
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
//	@Failure		404	{object}	errors.APIError	"Génération non trouvée"
//	@Failure		500	{object}	errors.APIError	"Erreur lors de la génération"
//	@Router			/courses/generate [post]
func (c courseController) GenerateCourse(ctx *gin.Context) {
	courseGenerateDTO := dto.GenerateCourseInput{}

	bindError := ctx.BindJSON(&courseGenerateDTO)
	if bindError != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Impossible de parser le json: "+bindError.Error())
		return
	}

	id, err := uuid.Parse(courseGenerateDTO.GenerationId)
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, "ID de génération invalide")
		return
	}

	if _, ok := c.ownedGeneration(ctx, id); !ok {
		return
	}

	result, err := c.service.GenerateCourseAsync(courseGenerateDTO)
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, "Erreur lors de la génération: "+err.Error())
		return
	}

	// Retourner un statut 202 (Accepted) pour indiquer le traitement asynchrone
	ctx.JSON(http.StatusAccepted, result)
}
