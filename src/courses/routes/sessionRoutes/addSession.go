package sessionController

import (
	"net/http"

	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

// Add Session godoc
//
//	@Summary		Création session
//	@Description	Ajoute une nouvelle session dans la base de données
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Param			session	body		dto.CreateSessionInput	true	"session"
//	@Success		201		{object}	dto.CreateSessionOutput
//
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400		{object}	errors.APIError	"Impossible de créer une session"
//	@Failure		409		{object}	errors.APIError	"La session existe déjà"
//	@Router			/sessions [post]
func (s sessionController) AddSession(ctx *gin.Context) {
	sessionCreateDTO := dto.CreateSessionInput{}

	bindError := ctx.BindJSON(&sessionCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	session, sessionError := s.service.CreateSession(sessionCreateDTO)

	if sessionError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: sessionError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, session)
}
