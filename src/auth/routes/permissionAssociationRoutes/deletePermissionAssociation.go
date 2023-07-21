package permissionAssociationController

import (
	"github.com/gin-gonic/gin"
)

// Delete permissionAssociation godoc
//
//	@Summary		Suppression permissionAssociation
//	@Description	Suppression d'un permissionAssociation dans la base de données
//	@Tags			permissionAssociations
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"ID permissionAssociation"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"PermissionAssociation non trouvée - Impossible de la supprimer "
//
//	@Router			/permissionAssociations/{id} [delete]
func (p permissionAssociationController) DeletePermissionAssociation(ctx *gin.Context) {

	p.DeleteEntity(ctx)
}
