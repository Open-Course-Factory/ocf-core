package courseController

import (
	"net/http"

	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

// Add Course godoc
//
// @Summary		Création cours
// @Description	Ajoute un nouveau cours dans la base de données
// @Tags		courses
// @Accept		json
// @Produce		json
// @Param		course	body		dto.CreateCourseInput	true	"cours"
//
// @Security Bearer
//
// @Success		201		{object}	dto.CreateCourseOutput
//
// @Failure		400		{object}	errors.APIError	"Impossible de parser le json"
// @Failure		400		{object}	errors.APIError	"Impossible de créer un cours"
// @Failure		409		{object}	errors.APIError	"Le cours existe déjà pour cet utilisateur"
// @Router			/courses [post]
func (c courseController) AddCourse(ctx *gin.Context) {
	courseCreateDTO := dto.CreateCourseInput{}

	bindError := ctx.BindJSON(&courseCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: bindError.Error(),
		})
		return
	}

	course, courseError := c.service.CreateCourse(courseCreateDTO)

	if courseError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: courseError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, course)
}
