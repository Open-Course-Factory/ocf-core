package permissionAssociationController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Edit permissionAssociation godoc
//
//	@Summary		Modification permissionAssociation
//	@Description	Modification d'un permissionAssociation dans la base de données
//	@Tags			permissionAssociations
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"ID permissionAssociation"
//	@Param 			permissionAssociation 	body	dto.PermissionAssociationEditInput	true	"PermissionAssociation"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		204	{object}	string
//
//	@Failure		403	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de récupérer l'ID permissionAssociation"
//	@Failure		400	{object}	errors.APIError	"Impossible de modifier la permissionAssociation"
//
//	@Router			/permissionAssociations/{id} [put]
func (p permissionAssociationController) EditPermissionAssociation(ctx *gin.Context) {

	editPermissionAssociation := &dto.PermissionAssociationEditInput{}
	id, errParse := uuid.Parse(ctx.Param("id"))

	errBind := ctx.BindJSON(&editPermissionAssociation)

	if errParse != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: errParse.Error(),
		})
		return
	}

	if errBind != nil {
		ctx.JSON(http.StatusForbidden,
			&errors.APIError{ErrorCode: http.StatusForbidden, ErrorMessage: errBind.Error()})
		return
	}

	edit, editPermissionAssociationError := p.service.EditPermissionAssociation(editPermissionAssociation, id)

	if editPermissionAssociationError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: editPermissionAssociationError.Error()})
		return
	}
	ctx.JSON(http.StatusNoContent, edit)
}
