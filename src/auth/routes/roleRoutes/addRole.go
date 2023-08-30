package roleController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Add Role godoc
//
//	@Summary		Création role
//	@Description	Ajoute un nouveau role dans la base de données
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//	@Param			role	body		dto.CreateRoleInput	true	"role"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		201		{object}	dto.RoleOutput
//
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400		{object}	errors.APIError	"Impossible de créer un role"
//	@Router			/roles [post]
func (roleController roleController) AddRole(ctx *gin.Context) {
	// permissionsArray, _, permissionsFound := controller.GetPermissionsFromContext(ctx)
	// if !permissionsFound {
	// 	return
	// }

	// isUserInstanceAdmin := (*roleController.GetPermissionService()).IsUserInstanceAdmin(permissionsArray)
	isUserInstanceAdmin := true
	if isUserInstanceAdmin {
		roleCreateDTO := dto.CreateRoleInput{}

		bindError := ctx.BindJSON(&roleCreateDTO)
		if bindError != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "Impossible de parser le json",
			})
			return
		}

		role, roleError := roleController.service.CreateRole(roleCreateDTO)

		if roleError != nil {
			ctx.JSON(http.StatusBadRequest, &errors.APIError{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: roleError.Error(),
			})
			return
		}

		ctx.JSON(http.StatusCreated, role)
		return
	} else {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Impossible de créer un role, seul l'administrateur de l'instance peut le faire",
		})
	}

}
