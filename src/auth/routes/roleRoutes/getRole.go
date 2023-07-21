package roleController

import (
	"github.com/gin-gonic/gin"
)

// Get role godoc
//
//	@Summary		Récupération role
//	@Description	Récupération des informations de l'role
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//	@Param		 	id	path		string	true	"ID role"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		200	{object}	dto.RoleOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Role inexistant - Impossible de le récupérer"
//
//	@Router			/roles/{id} [get]
func (roleController roleController) GetRole(ctx *gin.Context) {
	roleController.GetEntity(ctx)
}
