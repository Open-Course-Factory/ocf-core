package userController

import (
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
	u.GetEntities(ctx)
}
