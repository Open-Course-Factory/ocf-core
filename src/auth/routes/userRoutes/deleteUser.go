package userController

import (
	"github.com/gin-gonic/gin"
)

// Delete user godoc
//
//	@Summary		Suppression utilisateur
//	@Description	Suppression d'un utilisateur dans la base de données
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"ID utilisateur"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Utilisateur non trouvé - Impossible de le supprimer "
//
//	@Router			/users/{id} [delete]
func (u userController) DeleteUser(ctx *gin.Context) {

	u.GenericController.DeleteEntity(ctx)
}
