package groupController

import (
	"net/http"
	"soli/formations/src/courses/errors"

	"github.com/gin-gonic/gin"
)

// Delete group godoc
//
//	@Summary		Suppression groupe
//	@Description	Suppression d'un groupe dans la base de données
//	@Tags			groups
//	@Accept			json
//	@Produce		json
//	@Param			name	path	string	true	"Group name"
//
//	@Security		Bearer
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Group non trouvé - Impossible de le supprimer "
//
//	@Router			/groups/{name} [delete]
func (u groupController) DeleteGroup(ctx *gin.Context) {
	nameParam := ctx.Param("name")

	errorDelete := u.service.DeleteGroup(nameParam)
	if errorDelete != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Group not found",
		})
		ctx.Abort()
		return
	}

	//ToDo : handle error
	//casdoor.Enforcer.RemovePolicy(id.String())

	ctx.JSON(http.StatusNoContent, "Done")
}
