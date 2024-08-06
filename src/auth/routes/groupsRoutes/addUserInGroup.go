package groupController

import (
	"net/http"
	"soli/formations/src/auth/dto"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

// Add User in Group godoc
//
// @Summary		Création user dans un groupe
// @Description	Ajoute un nouvel user dans un groupe
// @Tags		groups
// @Accept		json
// @Produce		json
// @Param		input	body		dto.AddUserInGroupInput	true	"UserId and Group Name"
//
// @Security Bearer
//
// @Success		201		{object}	string
//
// @Failure		400		{object}	errors.APIError	"Impossible de parser le json"
// @Failure		400		{object}	errors.APIError	"Impossible d'ajouter un user"
// @Failure		409		{object}	errors.APIError	"L'utilisateur n'a pas pu être ajouté"
// @Router			/groups/{id} [patch]
func (g groupController) AddUserInGroup(ctx *gin.Context) {
	adduserInGroupDTO := dto.AddUserInGroupInput{}

	bindError := ctx.BindJSON(&adduserInGroupDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Impossible de parser le json",
		})
		return
	}

	result, addUserError := g.service.AddUserInGroup(adduserInGroupDTO)

	if addUserError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: addUserError.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, result)
}
