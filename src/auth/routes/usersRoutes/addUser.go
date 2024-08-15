package userController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Add User godoc
//
//	@Summary		Création user
//	@Description	Ajoute un nouvel user dans la base de données
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			user	body		dto.CreateUserInput	true	"user"
//
//	@Success		201		{object}	dto.CreateUserOutput
//
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400		{object}	errors.APIError	"Impossible de créer un user"
//	@Failure		409		{object}	errors.APIError	"Le user existe déjà"
//	@Router			/users [post]
func (u userController) AddUser(ctx *gin.Context) {
	userCreateDTO := dto.CreateUserInput{}

	bindError := ctx.BindJSON(&userCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	user, userError := u.service.AddUser(userCreateDTO)

	if userError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: userError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, user)
}
