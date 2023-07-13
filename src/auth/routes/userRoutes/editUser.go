package userController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
	"soli/formations/src/auth/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Edit user godoc
//
//	@Summary		Modification utilisateur (Admin)
//	@Description	Modification d'un utilisateur dans la base de données par un administrateur
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"ID utilisateur"
//	@Param 			user 	body	dto.UserEditInput	true	"Utilisateur"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		204	{object}	string
//
//	@Failure		403	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de récupérer l'ID utilisateur"
//	@Failure		400	{object}	errors.APIError	"Impossible de modifier l'utilisateur"
//
//	@Router			/users/{id} [put]
func (u userController) EditUser(ctx *gin.Context) {

	editUser := &dto.UserEditInput{}
	id, errParse := uuid.Parse(ctx.Param("id"))

	errBind := ctx.BindJSON(&editUser)

	if errParse != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: errParse.Error(),
		})
		return
	}

	if errBind != nil {
		ctx.JSON(http.StatusForbidden,
			&errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: errBind.Error()})
		return
	}

	edit, editUserError := u.service.EditUser(editUser, id, false)

	if editUserError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: editUserError.Error()})
		return
	}
	ctx.JSON(http.StatusNoContent, edit)
}

// Edit user self godoc
//
//	@Summary		Modification utilisateur
//	@Description	Modification des informations de l'utilisateur par lui même dans la base de données
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Success		204	{object}	string
//	@Param 			user 	body	dto.UserEditInput	true	"Utilisateur"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Failure		403	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Impossible de récupérer les informations de l'utilisateur"
//	@Failure		400	{object}	errors.APIError	"Impossible de modifier l'utilisateur"
//
//	@Router			/users [patch]
func (u userController) EditUserSelf(ctx *gin.Context) {

	editUser := &dto.UserEditInput{}

	rawUser, ok := ctx.Get("user")

	if !ok {
		return
	}

	userModel, isUser := rawUser.(*models.User)

	if !isUser {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Error check user"})
		return
	}

	errBind := ctx.BindJSON(&editUser)

	if errBind != nil {
		ctx.JSON(http.StatusForbidden,
			&errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: errBind.Error()})
		return
	}

	edit, editUserError := u.service.EditUser(editUser, userModel.ID, true)

	if editUserError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: editUserError.Error()})
		return
	}
	ctx.JSON(http.StatusNoContent, edit)
}
