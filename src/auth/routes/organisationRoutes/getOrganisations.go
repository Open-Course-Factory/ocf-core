package organisationController

import (
	"net/http"
	"soli/formations/src/auth/errors"

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
func (u organisationController) GetOrganisations(ctx *gin.Context) {

	organisations, err := u.service.GetOrganisations()

	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Organisation not found",
		})
		return
	}

	ctx.JSON(http.StatusOK, organisations)
}
