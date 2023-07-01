package roleController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Get role godoc
//
//	@Summary		Récupération role
//	@Description	Récupération des informations de l'role
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//	@Param		 	id	path		int	true	"ID role"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		200	{object}	dto.RoleOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Role inexistant - Impossible de le récupérer"
//
//	@Router			/roles/{id} [get]
func (roleController roleController) GetRole(ctx *gin.Context) {

	id, err := uuid.Parse(ctx.Param("id"))

	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	role, roleError := roleController.service.GetRole(id)

	if roleError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotAcceptable,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, dto.RoleModelToRoleOutput(*role))
}
