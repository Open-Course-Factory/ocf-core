package organisationController

import (
	"net/http"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Delete organisation godoc
//
//	@Summary		Suppression organisation
//	@Description	Suppression d'une organisation dans la base de données
//	@Tags			organisations
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"ID organisation"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		204	{object}	string
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Organisation non trouvée - Impossible de la supprimer "
//
//	@Router			/organisations/{id} [delete]
func (u organisationController) DeleteOrganisation(ctx *gin.Context) {

	id, parseError := uuid.Parse(ctx.Param("id"))
	if parseError != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: parseError.Error(),
		})
		return
	}

	errorDelete := u.service.DeleteOrganisation(id)
	if errorDelete != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Organisation not found",
		})
		return
	}
	ctx.JSON(http.StatusNoContent, "Done")
}
