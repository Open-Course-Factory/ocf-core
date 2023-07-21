package permissionAssociationController

import (
	"github.com/gin-gonic/gin"
)

// Get all permissionAssociations godoc
//
//	@Summary		Récupération permissionAssociations
//	@Description	Récupération de tous les permissionAssociations dans la base données
//	@Tags			permissionAssociations
//	@Accept			json
//	@Produce		json
//
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		200	{object}	[]dto.PermissionAssociationOutput
//
//	@Failure		404	{object}	errors.APIError	"PermissionAssociations inexistants"
//
//	@Router			/permissionAssociations [get]
func (p permissionAssociationController) GetPermissionAssociations(ctx *gin.Context) {

	p.GetEntities(ctx)
}
