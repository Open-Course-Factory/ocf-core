package courseController

import (
	"net/http"

	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

// Generate Course godoc
//
//	@Summary		Génération d'un cours
//	@Description	Génération d'un cours pour un format donné
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			course	body		dto.GenerateCourseInput	true	"cours"
//	@Param          Authorization   header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		201		{object}	dto.GenerateCourseOutput
//
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Router			/courses/generate [post]
func (c courseController) GenerateCourse(ctx *gin.Context) {
	courseGenerateDTO := dto.GenerateCourseInput{}

	bindError := ctx.BindJSON(&courseGenerateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	course, courseError := c.service.GenerateCourse(courseGenerateDTO.Name, courseGenerateDTO.Theme, courseGenerateDTO.Format, courseGenerateDTO.AuthorEmail)

	if courseError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: courseError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, course)
}
