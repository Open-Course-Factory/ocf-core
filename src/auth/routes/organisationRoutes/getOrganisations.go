package organisationController

import (
	"github.com/gin-gonic/gin"
)

// Get all organisations godoc
//
//	@Summary		Récupération organisations
//	@Description	Récupération de toutes les organisations dans la base données
//	@Tags			organisations
//	@Accept			json
//	@Produce		json
//
//	@Param Authorization header string true "Insert your access token" default(bearer <Add access token here>)
//
//	@Success		200	{object}	[]dto.OrganisationOutput
//
//	@Failure		404	{object}	errors.APIError	"Organisations inexistantes"
//
//	@Router			/organisations [get]
func (org organisationController) GetOrganisations(ctx *gin.Context) {

	org.GetEntitiesWithPermissionCheck(ctx)
}
