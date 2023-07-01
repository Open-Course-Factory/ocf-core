package permissionController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Add Permission godoc
//
//	@Summary		Création permission
//	@Description	Ajoute une nouvellepermission dans la base de données
//	@Tags			permissions
//	@Accept			json
//	@Produce		json
//	@Param			permission	body		dto.CreatePermissionInput	true	"permission"
//	@Success		201		{object}	dto.PermissionOutput
//
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400		{object}	errors.APIError	"Impossible de créer un permission"
//	@Router			/permissions [post]
func (p permissionController) AddPermission(ctx *gin.Context) {
	permissionCreateDTO := dto.CreatePermissionInput{}

	bindError := ctx.BindJSON(&permissionCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	user, userError := p.service.CreatePermission(permissionCreateDTO)

	if userError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: userError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, user)
}
