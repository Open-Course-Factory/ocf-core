package permissionAssociationController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

	id, err := uuid.Parse(ctx.Param("id"))

	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	permissionAssociation, permissionAssociationError := p.service.GetPermissionAssociation(id)

	if permissionAssociationError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotAcceptable,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.PermissionAssociationModelToPermissionAssociationOutput(*permissionAssociation))
}
