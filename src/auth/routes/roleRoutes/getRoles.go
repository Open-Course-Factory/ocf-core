package roleController

import (
	"net/http"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Get all roles godoc
//
//	@Summary		Récupération roles
//	@Description	Récupération de tous les roles dans la base données
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		200	{object}	[]dto.RoleOutput
//
//	@Failure		404	{object}	errors.APIError	"Roles inexistants"
//
//	@Router			/roles [get]
func (roleController roleController) GetRoles(ctx *gin.Context) {

	roles, err := roleController.service.GetRoles()

	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Role not found",
		})
		return
	}

	ctx.JSON(http.StatusOK, roles)
}
