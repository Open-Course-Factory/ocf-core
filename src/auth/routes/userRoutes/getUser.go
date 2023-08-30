package userController

import (
	"github.com/gin-gonic/gin"
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

	u.GetEntity(ctx)
}
