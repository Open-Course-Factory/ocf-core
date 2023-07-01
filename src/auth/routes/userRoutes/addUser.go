package userController

import (
	"net/http"
	"net/mail"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Add User godoc
//
//	@Summary		Création utilisateur
//	@Description	Ajoute un nouvel utilisateur dans la base de données
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			user	body		dto.CreateUserInput	true	"utilisateur"
//	@Success		201		{object}	dto.UserLoginOutput
//
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400		{object}	errors.APIError	"Adresse mail non conforme"
//	@Failure		400		{object}	errors.APIError	"Impossible de créer un utilisateur"
//	@Router			/users [post]
func (userController userController) AddUser(ctx *gin.Context) {
	userCreateDTO := dto.CreateUserInput{}

	bindError := ctx.BindJSON(&userCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	_, parseEmailError := mail.ParseAddress(userCreateDTO.Email)
	if parseEmailError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Adresse mail non conforme",
		})
		return
	}

	user, userError := userController.service.CreateUser(userCreateDTO, userController.config)

	if userError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: userError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, user)
}
