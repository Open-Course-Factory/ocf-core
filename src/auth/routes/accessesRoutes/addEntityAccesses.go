package accessController

import (
	"net/http"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

// Add Group User Accesses godoc
//
//	@Summary		Création des accès pour un groupe ou un utilisateur
//	@Description	Ajoute de nouveaux accès pour un groupe ou un utilisateur dans la base de données
//	@Tags			accesses
//	@Accept			json
//	@Produce		json
//	@Param			group_access	body	dto.CreateEntityAccessInput	true	"Accès pour un groupe ou un utilisateur"
//
//	@Security		Bearer
//
//	@Success		201	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		500	{object}	errors.APIError	"Impossible de créer l'accès"
//	@Router			/accesses [post]
func (u accessController) AddEntityAccesses(ctx *gin.Context) {
	groupAccessesCreateDTO := dto.CreateEntityAccessInput{}

	bindError := ctx.BindJSON(&groupAccessesCreateDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: bindError.Error(),
		})
		return
	}

	errPolicyLoading := casdoor.Enforcer.LoadPolicy()
	if errPolicyLoading != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: errPolicyLoading.Error(),
		})
		return
	}

	_, errPolicyDeleting := casdoor.Enforcer.RemovePolicy(groupAccessesCreateDTO.GroupName, groupAccessesCreateDTO.Route)

	if errPolicyDeleting != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: errPolicyDeleting.Error(),
		})
		return
	}

	_, errPolicyAdding := casdoor.Enforcer.AddPolicy(groupAccessesCreateDTO.GroupName, groupAccessesCreateDTO.Route, groupAccessesCreateDTO.AuthorizedMethods)

	if errPolicyAdding != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: errPolicyAdding.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, "Policy Added")
}
