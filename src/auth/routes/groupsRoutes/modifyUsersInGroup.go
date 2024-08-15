package groupController

import (
	"net/http"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Modify Users in Group godoc
//
//	@Summary		Modification d'utilisateurs dans un groupe
//	@Description	Modifie les utilisateurs dans un groupe
//	@Tags			groups
//	@Accept			json
//	@Produce		json
//	@Param			name	path	string						true	"Group name"
//	@Param			data	body	dto.ModifyUsersInGroupInput	true	"UserId and Action"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible d'ajouter un user"
//	@Failure		409	{object}	errors.APIError	"L'utilisateur n'a pas pu être ajouté"
//	@Router			/groups/{name} [patch]
func (g groupController) ModifyUsersInGroup(ctx *gin.Context) {
	nameParam := ctx.Param("name")
	modifyUsersInGroupDTO := dto.ModifyUsersInGroupInput{}

	bindError := ctx.BindJSON(&modifyUsersInGroupDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: bindError.Error(),
		})
		return
	}

	result, addUserError := g.service.ModifyUsersInGroup(nameParam, modifyUsersInGroupDTO)

	if addUserError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: addUserError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, result)
}
