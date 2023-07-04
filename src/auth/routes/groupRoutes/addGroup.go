package groupController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Add Group godoc
//
//	@Summary		Création groupe
//	@Description	Ajoute un nouveau groupe dans la base de données
//	@Tags			groups
//	@Accept			json
//	@Produce		json
//	@Param			group	body		dto.CreateGroupInput	true	"group"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		201		{object}	dto.GroupOutput
//
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400		{object}	errors.APIError	"Impossible de créer un groupe"
//	@Router			/groups [post]
func (g groupController) AddGroup(ctx *gin.Context) {
	groupCreateDTO := dto.CreateGroupInput{}

	bindError := ctx.BindJSON(&groupCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json : " + bindError.Error(),
		})
		return
	}

	user, userError := g.service.CreateGroup(groupCreateDTO)

	if userError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: userError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, user)
}
