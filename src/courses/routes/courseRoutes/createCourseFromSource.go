package courseController

import (
	"net/http"

	"soli/formations/src/courses/dto"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Create Course from Source godoc
//
//	@Summary		Création cours à partir d'une source (git ou locale)
//	@Description	Ajoute un nouveau cours dans la base de données à partir d'un dépôt git ou d'un chemin local
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			courseSource	body	dto.CreateCourseFromSourceInput	true	"cours"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.CreateCourseFromSourceOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer un cours"
//	@Failure		409	{object}	errors.APIError	"Cours déjà présent pour cet utilisateur"
//	@Router			/courses/source [post]
func (c courseController) CreateCourseFromSource(ctx *gin.Context) {
	createCourseFromSourceDTO := dto.CreateCourseFromSourceInput{}

	bindError := ctx.BindJSON(&createCourseFromSourceDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json: " + bindError.Error(),
		})
		return
	}

	// Validate source type
	if createCourseFromSourceDTO.SourceType != "git" && createCourseFromSourceDTO.SourceType != "local" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Source type must be 'git' or 'local'",
		})
		return
	}

	// For git sources, default branch to "main" if not provided
	branch := createCourseFromSourceDTO.Branch
	if createCourseFromSourceDTO.SourceType == "git" && branch == "" {
		branch = "main"
	}

	userId := ctx.GetString("userId")

	_, errGetCourse := c.service.GetCourse(userId, createCourseFromSourceDTO.Name, createCourseFromSourceDTO.SourceType, createCourseFromSourceDTO.Source, branch, "course.json")

	if errGetCourse != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de récupérer le cours : " + errGetCourse.Error(),
		})
		return
	}

	//ToDo: generate outputDto

	ctx.JSON(http.StatusCreated, nil)
}
