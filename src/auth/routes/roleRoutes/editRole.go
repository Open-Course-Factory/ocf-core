package roleController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Edit role godoc
//
//	@Summary		Modification role
//	@Description	Modification d'un role dans la base de données
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"ID role"
//	@Param 			role 	body	dto.RoleEditInput	true	"Role"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		204	{object}	string
//
//	@Failure		403	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de récupérer l'ID role"
//	@Failure		400	{object}	errors.APIError	"Impossible de modifier le role"
//
//	@Router			/roles/{id} [put]
func (roleController roleController) EditRole(ctx *gin.Context) {

	editRole := &dto.RoleEditInput{}
	id, errParse := uuid.Parse(ctx.Param("id"))

	errBind := ctx.BindJSON(&editRole)

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

	edit, editRoleError := roleController.service.EditRole(editRole, id)

	if editRoleError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: editRoleError.Error()})
		return
	}
	ctx.JSON(http.StatusNoContent, edit)
}
