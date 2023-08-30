package organisationController

import (
	"github.com/gin-gonic/gin"
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
func (org organisationController) DeleteOrganisation(ctx *gin.Context) {
	org.DeleteEntity(ctx)
}
