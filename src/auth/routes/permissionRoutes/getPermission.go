package permissionController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Get permission godoc
//
//	@Summary		Récupération permission
//	@Description	Récupération des informations de la permission
//	@Tags			permissions
//	@Accept			json
//	@Produce		json
//	@Param		 	id	path		int	true	"ID permission"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		200	{object}	dto.PermissionOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Permission inexistante - Impossible de la récupérer"
//
//	@Router			/permissions/{id} [get]
func (p permissionController) GetPermission(ctx *gin.Context) {

	id, err := uuid.Parse(ctx.Param("id"))

	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	permission, permissionError := p.service.GetPermission(id)

	if permissionError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotAcceptable,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.PermissionModelToPermissionOutput(*permission))
}
