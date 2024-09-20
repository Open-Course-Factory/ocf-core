package courseController

import (
	"net/http"

	"soli/formations/src/courses/dto"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Create Course from Git godoc
//
//	@Summary		Création cours à partir d'un dépôt git
//	@Description	Ajoute un nouveau cours dans la base de données
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			gitRepository	body	dto.CreateCourseFromGitInput	true	"cours"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.CreateCourseFromGitOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer un cours"
//	@Failure		409	{object}	errors.APIError	"Cours déjà présent pour cet utilisateur"
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

	userId := ctx.GetString("userId")

	_, errGetCourse := c.service.GetGitCourse(userId, createCourseFromGitDTO.Name, createCourseFromGitDTO.Url, createCourseFromGitDTO.BranchName)

	if errGetCourse != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de récupérer le cours",
		})
		return
	}

	//ToDo: generate outputDto

	ctx.JSON(http.StatusCreated, nil)
}
