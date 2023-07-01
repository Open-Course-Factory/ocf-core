package permissionController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Edit permission godoc
//
//	@Summary		Modification permission
//	@Description	Modification d'un permission dans la base de données
//	@Tags			permissions
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"ID permission"
//	@Param 			permission 	body	dto.PermissionEditInput	true	"Permission"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		204	{object}	string
//
//	@Failure		403	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de récupérer l'ID permission"
//	@Failure		400	{object}	errors.APIError	"Impossible de modifier la permission"
//
//	@Router			/permissions/{id} [put]
func (p permissionController) EditPermission(ctx *gin.Context) {

	editPermission := &dto.PermissionEditInput{}
	id, errParse := uuid.Parse(ctx.Param("id"))

	errBind := ctx.BindJSON(&editPermission)

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

	edit, editPermissionError := p.service.EditPermission(editPermission, id)

	if editPermissionError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: editPermissionError.Error()})
		return
	}
	ctx.JSON(http.StatusNoContent, edit)
}
