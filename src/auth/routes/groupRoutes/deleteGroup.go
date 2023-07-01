package groupController

import (
	"net/http"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Delete user godoc
//
//	@Summary		Suppression groupe
//	@Description	Suppression d'un groupe dans la base de données
//	@Tags			groups
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"ID group"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Groupe non trouvé - Impossible de le supprimer "
//
//	@Router			/groups/{id} [delete]
func (g groupController) DeleteGroup(ctx *gin.Context) {

	id, parseErr := uuid.Parse(ctx.Param("id"))
	if parseErr != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: parseErr.Error(),
		})
		return
	}

	errorDelete := g.service.DeleteGroup(id)
	if errorDelete != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Group not found",
		})
		return
	}
	ctx.JSON(http.StatusNoContent, "Done")
}
