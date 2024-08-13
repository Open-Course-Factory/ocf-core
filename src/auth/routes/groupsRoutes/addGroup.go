package groupController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

// Add Group godoc
//
//	@Summary		Création group
//	@Description	Ajoute un nouveau group dans la base de données
//	@Tags			groups
//	@Accept			json
//	@Produce		json
//	@Param			group	body	dto.CreateGroupInput	true	"group"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.CreateGroupOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer un groupe"
//	@Failure		409	{object}	errors.APIError	"Le groupe existe déjà"
//	@Router			/groups [post]
func (u groupController) AddGroup(ctx *gin.Context) {
	groupCreateDTO := dto.CreateGroupInput{}

	bindError := ctx.BindJSON(&groupCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	user, userError := u.service.AddGroup(groupCreateDTO)

	if userError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: userError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, user)
}
