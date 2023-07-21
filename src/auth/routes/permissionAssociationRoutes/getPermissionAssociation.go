package permissionAssociationController

import (
	"github.com/gin-gonic/gin"
)

// Get permissionAssociation godoc
//
//	@Summary		Récupération permissionAssociation
//	@Description	Récupération des informations de la permissionAssociation
//	@Tags			permissionAssociations
//	@Accept			json
//	@Produce		json
//	@Param		 	id	path		int	true	"ID permissionAssociation"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		200	{object}	dto.PermissionAssociationOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"PermissionAssociation inexistante - Impossible de la récupérer"
//
//	@Router			/permissionAssociations/{id} [get]
func (p permissionAssociationController) GetPermissionAssociation(ctx *gin.Context) {

	p.GetEntity(ctx)
}
