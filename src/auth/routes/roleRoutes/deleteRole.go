package roleController

import (
	"net/http"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Delete role godoc
//
//	@Summary		Suppression role
//	@Description	Suppression d'un role dans la base de données
//	@Tags			roles
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"ID role"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Role non trouvé - Impossible de le supprimer "
//
//	@Router			/roles/{id} [delete]
func (roleController roleController) DeleteRole(ctx *gin.Context) {

	id, parseErr := uuid.Parse(ctx.Param("id"))
	if parseErr != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: parseErr.Error(),
		})
		return
	}

	errorDelete := roleController.service.DeleteRole(id)
	if errorDelete != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Role not found",
		})
		return
	}
	ctx.JSON(http.StatusNoContent, "Done")
}
