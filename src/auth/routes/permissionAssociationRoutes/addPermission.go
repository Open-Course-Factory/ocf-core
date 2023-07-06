package permissionAssociationController

import (
	"net/http"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Add PermissionAssociation godoc
//
//	@Summary		Création permissionAssociation
//	@Description	Ajoute une nouvelle permissionAssociation dans la base de données
//	@Tags			permissionAssociations
//	@Accept			json
//	@Produce		json
//	@Param			permissionAssociation	body		dto.CreatePermissionAssociationInput	true	"permissionAssociation"
//	@Success		201		{object}	dto.PermissionAssociationOutput
//
//	@Failure		400		{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400		{object}	errors.APIError	"Impossible de créer un permissionAssociation"
//	@Router			/permissionAssociations [post]
func (p permissionAssociationController) AddPermissionAssociation(ctx *gin.Context) {
	permissionAssociationCreateDTO := dto.CreatePermissionAssociationInput{}

	bindError := ctx.BindJSON(&permissionAssociationCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	user, userError := p.service.CreatePermissionAssociation(permissionAssociationCreateDTO)

	if userError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: userError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, user)
}
