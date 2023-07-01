package userController

import (
	"net/http"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Get all users godoc
//
//	@Summary		Récupération utilisateurs
//	@Description	Récupération de tous les utilisateurs dans la base données
//	@Tags			users
//	@Accept			json
//	@Produce		json
//
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		200	{object}	[]dto.UserOutput
//
//	@Failure		404	{object}	errors.APIError	"Utilisateurs inexistants"
//
//	@Router			/users [get]
func (u userController) GetUsers(ctx *gin.Context) {

	users, err := u.service.GetUsers()

	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "User not found",
		})
		return
	}

	ctx.JSON(http.StatusOK, users)
}
