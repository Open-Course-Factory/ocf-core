package userController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Get user godoc
//
//	@Summary		Récupération utilisateur
//	@Description	Récupération des informations de l'utilisateur
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param		 	id	path		string	true	"ID utilisateur"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		200	{object}	dto.UserOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Utilisateur inexistant - Impossible de le récupérer"
//
//	@Router			/users/{id} [get]
func (u userController) GetUser(ctx *gin.Context) {

	id, err := uuid.Parse(ctx.Param("id"))

	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	user, userError := u.service.GetUser(id)

	if userError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotAcceptable,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.UserModelToUserOutput(*user))
}
