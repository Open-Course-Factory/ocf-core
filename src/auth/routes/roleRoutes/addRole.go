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
//	@Success		201		{object}	dto.RoleOutput
//
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400		{object}	errors.APIError	"Impossible de créer un role"
//	@Router			/roles [post]
func (roleController roleController) AddRole(ctx *gin.Context) {
	roleCreateDTO := dto.CreateRoleInput{}

	bindError := ctx.BindJSON(&roleCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	role, roleError := roleController.service.CreateRole(roleCreateDTO, roleController.config)

	if roleError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: roleError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, role)
}
