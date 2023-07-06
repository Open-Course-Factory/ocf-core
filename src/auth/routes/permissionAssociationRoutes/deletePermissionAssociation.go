package permissionAssociationController

import (
	"net/http"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

	id, parseErr := uuid.Parse(ctx.Param("id"))
	if parseErr != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: parseErr.Error(),
		})
		return
	}

	errorDelete := p.service.DeletePermissionAssociation(id)
	if errorDelete != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "PermissionAssociation not found",
		})
		return
	}
	ctx.JSON(http.StatusNoContent, "Done")
}
