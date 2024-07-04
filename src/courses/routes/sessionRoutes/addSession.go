package sessionController

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

var _ = errors.APIError{}
var _ = dto.CreateSessionInput{}

// Add Session godoc
//
// @Summary		Création session
// @Description	Ajoute une nouvelle session dans la base de données
// @Tags			sessions
// @Accept			json
// @Produce		json
// @Param			session	body		dto.CreateSessionInput	true	"session"
//
// @Security Bearer
//
// @Success		201		{object}	dto.CreateSessionOutput
//
// @Failure		400		{object}	errors.APIError	"Impossible de parser le json"
// @Failure		400		{object}	errors.APIError	"Impossible de créer une session"
// @Failure		409		{object}	errors.APIError	"La session existe déjà"
// @Router			/sessions [post]
func (s sessionController) AddSession(ctx *gin.Context) {
	s.AddEntity(ctx)
}
