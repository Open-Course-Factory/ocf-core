package organisationController

import (
	"github.com/gin-gonic/gin"
)

// Get organisation godoc
//
//	@Summary		Récupération organisation
//	@Description	Récupération des informations de l'organisation
//	@Tags			organisations
//	@Accept			json
//	@Produce		json
//	@Param		 	id	path		string	true	"ID organisation"
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//	@Success		200	{object}	dto.OrganisationOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		404	{object}	errors.APIError	"Organisation inexistante - Impossible de la récupérer"
//
//	@Router			/organisations/{id} [get]
func (org organisationController) GetOrganisation(ctx *gin.Context) {

	org.GetEntity(ctx)
}
