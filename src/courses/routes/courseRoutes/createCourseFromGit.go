package courseController

import (
	"net/http"

	"soli/formations/src/courses/dto"

	"soli/formations/src/auth/errors"
	authModels "soli/formations/src/auth/models"

	"github.com/gin-gonic/gin"
)

// Create Course from Git godoc
//
//	@Summary		Création cours à partir d'un dépôt git
//	@Description	Ajoute un nouveau cours dans la base de données
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			gitRepository	body		dto.CreateCourseFromGitInput	true	"cours"
//	@Param          Authorization   header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		201		{object}	dto.CreateCourseFromGitOutput
//
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400		{object}	errors.APIError	"Impossible de créer un cours"
//	@Failure		409		{object}	errors.APIError	"Cours déjà présent pour cet utilisateur"
//	@Router			/courses/git [post]
func (c courseController) CreateCourseFromGit(ctx *gin.Context) {
	createCourseFromGitDTO := dto.CreateCourseFromGitInput{}

	bindError := ctx.BindJSON(&createCourseFromGitDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	rawUser, ok := ctx.Get("user")

	if !ok {
		return
	}

	user, err := c.GenericService.GetEntity(rawUser.(*authModels.User).ID, authModels.User{})

	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de récupérer l'utilisateur",
		})
	}

	courseOutput, errGetCourse := c.service.GetGitCourse(*user.(*authModels.User), createCourseFromGitDTO.Url)

	if errGetCourse != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de récupérer le cours",
		})
		return
	}

	if courseOutput == nil {
		ctx.JSON(http.StatusConflict, &errors.APIError{
			ErrorCode:    http.StatusConflict,
			ErrorMessage: "Le cours existe déjà pour cet utilisateur",
		})
		return
	}

	ctx.JSON(http.StatusCreated, courseOutput)
}
