package accessController

import (
	"net/http"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
	"soli/formations/src/utils"

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

	// Prepare permission options with LoadPolicyFirst
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true

	// Remove existing policy if present
	errPolicyDeleting := utils.RemovePolicy(casdoor.Enforcer, groupAccessesCreateDTO.GroupName, groupAccessesCreateDTO.Route, "", opts)
	if errPolicyDeleting != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: errPolicyDeleting.Error(),
		})
		return
	}

	// Add new policy
	errPolicyAdding := utils.AddPolicy(casdoor.Enforcer, groupAccessesCreateDTO.GroupName, groupAccessesCreateDTO.Route, groupAccessesCreateDTO.AuthorizedMethods, opts)
	if errPolicyAdding != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: errPolicyAdding.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, "Policy Added")
}
