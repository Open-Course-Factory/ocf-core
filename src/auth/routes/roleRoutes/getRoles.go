package roleController

import (
	"github.com/gin-gonic/gin"
)

// Get all roles godoc
//
//	@Summary		Récupération roles
//	@Description	Récupération de tous les roles dans la base données
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		200	{object}	[]dto.RoleOutput
//
//	@Failure		404	{object}	errors.APIError	"Roles inexistants"
//
//	@Router			/roles [get]
func (roleController roleController) GetRoles(ctx *gin.Context) {
	roleController.GenericController.GetEntities(ctx)
}
