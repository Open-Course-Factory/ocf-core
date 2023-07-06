package permissionAssociationController

import (
	"net/http"
	"soli/formations/src/auth/errors"

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

	permissionAssociations, err := p.service.GetPermissionAssociations()

	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "PermissionAssociation not found",
		})
		return
	}

	ctx.JSON(http.StatusOK, permissionAssociations)
}
