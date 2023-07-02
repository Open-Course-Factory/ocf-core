package userController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"

	"github.com/gin-gonic/gin"
)

// Add user ssh key godoc
//
//	@Summary		Ajout d'une clé SSH à l'tilisateur courant
//	@Description	Ajout d'une clé SSH à l'tilisateur courant
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param 			sshKey 	body	dto.CreateSshKeyInput	true	"Clé SSH"
//	@Param 			Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		201	{object}	string
//
//	@Failure		403	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Impossible de récupérer l'ID utilisateur"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer la clé "
//
//	@Router			/users/sshkey [post]
func (u userController) AddUserSshKey(ctx *gin.Context) {

	rawUser, ok := ctx.Get("user")

	if !ok {
		return
	}

	userModel, isUser := rawUser.(*models.User)

	if !isUser {
		return
	}

	addUserSshKey := &dto.CreateSshKeyInput{}

	errBind := ctx.BindJSON(&addUserSshKey)

	if errBind != nil {
		return
	}

	addUserSshKey.UserId = userModel.ID

	userSshKeyAdded, addUserSshKeyError := u.service.AddUserSshKey(*addUserSshKey)

	if addUserSshKeyError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: addUserSshKeyError.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, userSshKeyAdded)
}
