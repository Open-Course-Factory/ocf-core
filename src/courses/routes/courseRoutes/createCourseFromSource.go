package courseController

import (
	"net/http"

	"soli/formations/src/courses/dto"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Create Course from Source godoc
//
//	@Summary		Création cours à partir d'un dépôt git
//	@Description	Ajoute un nouveau cours dans la base de données à partir d'un dépôt git (les sources locales sont réservées au CLI)
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
//	@Failure		400	{object}	errors.APIError	"Source type must be 'git'"
//	@Failure		409	{object}	errors.APIError	"Cours déjà présent pour cet utilisateur"
//	@Router			/courses/source [post]
func (c courseController) CreateCourseFromSource(ctx *gin.Context) {
	createCourseFromSourceDTO := dto.CreateCourseFromSourceInput{}

	bindError := ctx.BindJSON(&createCourseFromSourceDTO)
	if bindError != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Impossible de parser le json: "+bindError.Error())
		return
	}

	// Local sources read the server's filesystem: CLI only (src/cli calls the service directly)
	if createCourseFromSourceDTO.SourceType != "git" {
		errors.Respond(ctx, http.StatusBadRequest, "Source type must be 'git'")
		return
	}

	branch := createCourseFromSourceDTO.Branch
	if branch == "" {
		branch = "main"
	}

	userId := ctx.GetString("userId")

	_, errGetCourse := c.service.GetCourse(userId, createCourseFromSourceDTO.Name, createCourseFromSourceDTO.SourceType, createCourseFromSourceDTO.Source, branch, "course.json")

	if errGetCourse != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Impossible de récupérer le cours : "+errGetCourse.Error())
		return
	}

	//ToDo: generate outputDto

	ctx.JSON(http.StatusCreated, nil)
}
