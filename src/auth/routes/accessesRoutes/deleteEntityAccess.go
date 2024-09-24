package accessController

import (
	"net/http"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Delete group user access godoc
//
//	@Summary		Suppression d'accès pour un groupe ou un utilisateur
//	@Description	Suppression d'un accès pour un groupe ou un utilisateur dans la base de données
//	@Tags			accesses
//	@Accept			json
//	@Produce		json
//	@Param			entity_access	body	dto.DeleteEntityAccessInput	true	"Accès à révoquer pour un groupe ou un utilisateur"
//
//	@Security		Bearer
//
//	@Success		204	string		""
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Entité non trouvée - Impossible de supprimer "
//	@Failure		500	{object}	errors.APIError	"Impossible de supprimer l'accès"
//
//	@Router			/accesses [delete]
func (u accessController) DeleteEntityAccesses(ctx *gin.Context) {
	groupAccessesDeleteDTO := dto.DeleteEntityAccessInput{}

	bindError := ctx.BindJSON(&groupAccessesDeleteDTO)
	if bindError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: bindError.Error(),
		})
		return
	}

	errPolicyLoading := casdoor.Enforcer.LoadPolicy()
	if errPolicyLoading != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: errPolicyLoading.Error(),
		})
		return
	}

	_, errPolicyDeleting := casdoor.Enforcer.RemovePolicy(groupAccessesDeleteDTO.GroupName, groupAccessesDeleteDTO.Route)

	if errPolicyDeleting != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: errPolicyDeleting.Error(),
		})
		ctx.Abort()
		return
	}

	ctx.JSON(http.StatusNoContent, nil)
}
